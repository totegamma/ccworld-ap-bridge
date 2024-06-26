package activitypub

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/core"
	"github.com/totegamma/concurrent/x/message"
)

func (h *Handler) StartMessageWorker() {

	ticker10 := time.NewTicker(10 * time.Second)
	workers := make(map[string]context.CancelFunc)

	for {
		<-ticker10.C
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		jobs, err := h.repo.GetAllFollowers(ctx)
		if err != nil {
			log.Printf("error: %v", err)
		}

		for _, job := range jobs {
			if _, ok := workers[job.ID]; !ok {
				log.Printf("start worker %v", job.ID)
				ctx, cancel := context.WithCancel(context.Background())
				workers[job.ID] = cancel

				entity, err := h.repo.GetEntityByID(ctx, job.PublisherUserID)
				if err != nil {
					log.Printf("error: %v", err)
				}
				ownerID := entity.CCID
				home := entity.HomeStream
				if home == "" {
					continue
				}
				if entity.MovedTo != "" {
					continue
				}
				pubsub := h.rdb.Subscribe(ctx)
				pubsub.Subscribe(ctx, home)

				go func(ctx context.Context, job ApFollower) {
					for {
						select {
						case <-ctx.Done():
							log.Printf("worker %v done", job.ID)
							return
						default:
							pubsubMsg, err := pubsub.ReceiveMessage(ctx)
							if ctx.Err() != nil {
								continue
							}
							if err != nil {
								log.Printf("error: %v", err)
								continue
							}

							log.Printf("[worker %v] message received!\n", job.ID)

							var streamEvent core.Event
							err = json.Unmarshal([]byte(pubsubMsg.Payload), &streamEvent)
							if err != nil {
								log.Printf("error: %v", err)
								continue
							}

							messageID, ok := streamEvent.Body.(map[string]interface{})["id"].(string)
							if !ok {
								log.Printf("streamEvent body read id failed: %v", streamEvent.Body)
								continue
							}

							messageAuthor, ok := streamEvent.Body.(map[string]interface{})["author"].(string)
							if !ok {
								log.Printf("streamEvent body read author failed: %v", streamEvent.Body)
								continue
							}

							if messageAuthor != ownerID {
								log.Printf("message author is not owner: %v", messageAuthor)
								continue
							}

							note, err := h.MessageToNote(ctx, messageID)
							if err != nil {
								log.Printf("error: %v", err)
								continue
							}

							if note.Type == "Announce" {
								announce := Object{
									Context: []string{"https://www.w3.org/ns/activitystreams"},
									Type:    "Announce",
									ID:      "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageID + "/activity",
									Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + job.PublisherUserID,
									Content: "",
									Object:  note.Object,
									To:      []string{"https://www.w3.org/ns/activitystreams#Public"},
								}

								err = h.PostToInbox(ctx, job.SubscriberInbox, announce, entity)
								if err != nil {
									log.Printf("error: %v", err)
									continue
								}
								log.Printf("[worker %v] created", job.ID)
							} else {

								create := Create{
									Context: []string{"https://www.w3.org/ns/activitystreams"},
									Type:    "Create",
									ID:      "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageID + "/activity",
									Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + job.PublisherUserID,
									To:      []string{"https://www.w3.org/ns/activitystreams#Public"},
									Object:  note,
								}

								err = h.PostToInbox(ctx, job.SubscriberInbox, create, entity)
								if err != nil {
									log.Printf("error: %v", err)
									continue
								}
								log.Printf("[worker %v] created", job.ID)
							}
						}
					}
				}(ctx, job)
			}
		}

		// create job id list
		var jobIDs []string
		for _, job := range jobs {
			jobIDs = append(jobIDs, job.ID)
		}

		for routineID, cancel := range workers {
			if !isInList(routineID, jobIDs) {
				log.Printf("cancel worker %v", routineID)
				cancel()
				delete(workers, routineID)
			}
		}
	}
}

func (h *Handler) StartAssociationWorker(notificationStream string) {

	ctx := context.Background()
	pubsub := h.rdb.Subscribe(ctx)
	pubsub.Subscribe(ctx, notificationStream)

	for {
		pubsubMsg, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}

		log.Printf("received association: %v", pubsubMsg.Payload)

		var streamEvent core.Event
		err = json.Unmarshal([]byte(pubsubMsg.Payload), &streamEvent)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}

		associationID, ok := streamEvent.Body.(map[string]interface{})["id"].(string)
		if !ok {
			log.Printf("streamEvent body read id failed: %v", streamEvent.Body)
			continue
		}

		ass, err := h.association.Get(ctx, associationID)
		if err != nil {
			log.Printf("error: %v", err)
		}

		if ass.TargetType != "messages" {
			continue
		}

		assauthor, err := h.repo.GetEntityByCCID(ctx, ass.Author)
		if err != nil {
			log.Printf("get ass author entity failed: %v", err)
			continue
		}

		if assauthor.MovedTo != "" {
			continue
		}

		var associationObj association.SignedObject
		err = json.Unmarshal([]byte(ass.Payload), &associationObj)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}

		body, ok := associationObj.Body.(map[string]interface{})
		if !ok {
			log.Printf("parse association body failed")
			continue
		}

		msg, err := h.message.Get(ctx, ass.TargetID, h.apconfig.ProxyCCID)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}

		var msgObject message.SignedObject
		err = json.Unmarshal([]byte(msg.Payload), &msgObject)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}

		msgMeta, ok := msgObject.Meta.(map[string]interface{})
		ref, ok := msgMeta["apObjectRef"].(string)
		if !ok {
			log.Printf("target Message is not activitypub message")
			continue
		}
		dest, ok := msgMeta["apPublisherInbox"].(string)
		if !ok {
			log.Printf("target Message is not activitypub message")
			continue
		}

		if associationObj.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/like/0.0.1.json" ||
			associationObj.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/emoji/0.0.1.json" { // Like or Emoji
			shortcode, ok := body["shortcode"].(string)
			if ok {
				shortcode = ":" + shortcode + ":"
			} else {
				shortcode = "⭐"
			}

			var tag []Tag
			imageUrl, ok := body["imageUrl"].(string)
			if ok {
				tag = []Tag{
					{
						Type: "Emoji",
						ID:   imageUrl,
						Name: shortcode,
						Icon: Icon{
							Type:      "Image",
							MediaType: "image/png",
							URL:       imageUrl,
						},
					},
				}
			}

			like := Object{
				Context: []string{"https://www.w3.org/ns/activitystreams"},
				Type:    "Like",
				ID:      "https://" + h.config.Concurrent.FQDN + "/ap/likes/" + ass.ID,
				Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
				Content: shortcode,
				Tag:     tag,
				Object:  ref,
			}

			err = h.PostToInbox(ctx, dest, like, assauthor)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}
		} else if associationObj.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/reply/0.0.1.json" { // reply
			messageId, ok := body["messageId"].(string)
			if !ok {
				continue
			}
			// messageAuthor, ok := body["messageAuthor"].(string)
			// if !ok {
			// 	continue
			// }

			reply, err := h.message.Get(ctx, messageId, h.apconfig.ProxyCCID)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}

			var signedObject message.SignedObject
			err = json.Unmarshal([]byte(reply.Payload), &signedObject)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}

			replyBody, ok := signedObject.Body.(map[string]interface{})
			if !ok {
				continue
			}

			content, ok := replyBody["body"].(string)
			if !ok || content == "" {
				continue
			}

			create := Object{
				Context: []string{"https://www.w3.org/ns/activitystreams"},
				Type:    "Create",
				ID:      "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageId + "/activity",
				Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
				Object: Note{
					Type:         "Note",
					ID:           "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageId,
					AttributedTo: "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
					Content:      content,
					InReplyTo:    ref,
					To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
				},
			}

			err = h.PostToInbox(ctx, dest, create, assauthor)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}

		} else if associationObj.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/reroute/0.0.1.json" { // boost
			messageId, ok := body["messageId"].(string)
			if !ok {
				continue
			}
			// messageAuthor, ok := body["messageAuthor"].(string)
			// if !ok {
			// 	continue
			// }

			reply, err := h.message.Get(ctx, messageId, h.apconfig.ProxyCCID)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}

			var signedObject message.SignedObject
			err = json.Unmarshal([]byte(reply.Payload), &signedObject)
			if err != nil {
				log.Printf("error: %v", err)
				continue
			}

			replyBody, ok := signedObject.Body.(map[string]interface{})
			if !ok {
				continue
			}

			content, ok := replyBody["body"].(string)
			if !ok {
				content = ""
			}

			if content == "" { // boost
				announce := Object{
					Context: []string{"https://www.w3.org/ns/activitystreams"},
					Type:    "Announce",
					ID:      "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageId,
					Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
					Content: "",
					Object:  ref,
				}
				err = h.PostToInbox(ctx, dest, announce, assauthor)
				if err != nil {
					log.Printf("error: %v", err)
					continue
				}
			} else { // quote
				create := Object{
					Context: []string{"https://www.w3.org/ns/activitystreams"},
					Type:    "Create",
					ID:      "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageId + "/activity",
					Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
					Object: Note{
						Type:         "Note",
						ID:           "https://" + h.config.Concurrent.FQDN + "/ap/note/" + messageId,
						AttributedTo: "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + assauthor.ID,
						Content:      content,
						QuoteURL:     ref,
						To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
					},
				}

				err = h.PostToInbox(ctx, dest, create, assauthor)
				if err != nil {
					log.Printf("error: %v", err)
					continue
				}
			}
		} else {
			continue
		}
	}
}

func isInList(server string, list []string) bool {
	for _, s := range list {
		if s == server {
			return true
		}
	}
	return false
}
