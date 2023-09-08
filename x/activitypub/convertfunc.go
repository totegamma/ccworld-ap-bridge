package activitypub

import (
	"time"
	"errors"
	"regexp"
	"context"
	"encoding/json"
	"github.com/totegamma/concurrent/x/message"
)

func (h Handler) MessageToNote(ctx context.Context, messageID string) (Note, error) {
	ctx, span := tracer.Start(ctx, "MessageToNote")
	defer span.End()

	msg, err := h.message.Get(ctx, messageID)
	if err != nil {
		span.RecordError(err)
		return Note{}, errors.New("message not found")
	}

	entity, err := h.repo.GetEntityByCCID(ctx, msg.Author)
	if err != nil {
		span.RecordError(err)
		return Note{}, errors.New("entity not found")
	}


	var signedObject message.SignedObject
	err = json.Unmarshal([]byte(msg.Payload), &signedObject)
	if err != nil {
		return Note{}, errors.New("invalid payload")
	}

	body, ok := signedObject.Body.(map[string]interface{})
	if !ok {
		return Note{}, errors.New("invalid body")
	}

	var emojis []Tag
	var images []string

	text, ok := body["body"].(string)
	if !ok {
		return Note{}, errors.New("invalid body")
	}

	// extract image url of markdown notation
	imagePattern := regexp.MustCompile(`!\[.*\]\((.*)\)`)
	matches := imagePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		images = append(images, match[1])
	}

	// remove markdown notation
	text = imagePattern.ReplaceAllString(text, "")

	e, ok := body["emojis"].(map[string]interface{})
	if ok {
		for k, v := range e {
			imageURL, ok := v.(map[string]interface{})["imageURL"].(string)
			if !ok {
				continue
			}
			emoji := Tag{
				ID:   imageURL,
				Type: "Emoji",
				Name: ":" + k + ":",
				Icon: Icon{
					Type:      "Image",
					MediaType: "image/png",
					URL:       imageURL,
				},
			}
			emojis = append(emojis, emoji)
		}
	}

	attachments := []Attachment{}
	for _, imageURL := range images {
		attachment := Attachment{
			Type:      "Document",
			MediaType: "image/png",
			URL:       imageURL,
		}
		attachments = append(attachments, attachment)
	}

	fqdn := h.config.Concurrent.FQDN

	if signedObject.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/messages/note/0.0.1.json" { // Note
		return Note{
			Context:      "https://www.w3.org/ns/activitystreams",
			Type:         "Note",
			ID:           "https://" + fqdn + "/ap/note/" + msg.ID,
			AttributedTo: "https://" + fqdn + "/ap/acct/" + entity.ID,
			Content:      text,
			Published:    signedObject.SignedAt.Format(time.RFC3339),
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			Tag:          emojis,
			Attachment:   attachments,
		}, nil
	} else if signedObject.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/messages/reply/0.0.1.json" { // Reply
		sourceID, ok := body["replyToMessageId"].(string)
		if !ok {
			return Note{}, errors.New("invalid body")
		}

		replySource, err := h.message.Get(ctx, sourceID)
		if err != nil {
			span.RecordError(err)
			return Note{}, errors.New("message not found")
		}

		var replySignedObject message.SignedObject
		err = json.Unmarshal([]byte(replySource.Payload), &replySignedObject)
		if err != nil {
			return Note{}, errors.New("invalid payload")
		}

		replyMeta, ok := replySignedObject.Meta.(map[string]interface{})
		if !ok {
			return Note{}, errors.New("invalid meta")
		}

		ref, ok := replyMeta["apObjectRef"].(string)
		if !ok {
			ref = "https://" + fqdn + "/ap/note/" + sourceID
		}

		return Note{
			Context:      "https://www.w3.org/ns/activitystreams",
			Type:         "Note",
			ID:           "https://" + fqdn + "/ap/note/" + msg.ID,
			AttributedTo: "https://" + fqdn + "/ap/acct/" + entity.ID,
			Content:      text,
			InReplyTo:    ref,
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
		}, nil
	} else if signedObject.Schema == "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/messages/reroute/0.0.1.json" { // Boost or Quote
		sourceID, ok := body["rerouteMessageId"].(string)
		if !ok {
			return Note{}, errors.New("invalid body")
		}

		rerouteSource, err := h.message.Get(ctx, sourceID)
		if err != nil {
			span.RecordError(err)
			return Note{}, errors.New("message not found")
		}

		var rerouteSignedObject message.SignedObject
		err = json.Unmarshal([]byte(rerouteSource.Payload), &rerouteSignedObject)
		if err != nil {
			return Note{}, errors.New("invalid payload")
		}

		rerouteMeta, ok := rerouteSignedObject.Meta.(map[string]interface{})
		if !ok {
			return Note{}, errors.New("invalid meta")
		}

		ref, ok := rerouteMeta["apObjectRef"].(string)
		if !ok {
			ref = "https://" + fqdn + "/ap/note/" + sourceID
		}

		if text == "" {
			return Note{
				Context:      "https://www.w3.org/ns/activitystreams",
				Type:         "Announce",
				ID:           "https://" + fqdn + "/ap/note/" + msg.ID,
				Object:       ref,
			}, nil
		}

		return Note{
			Context:      "https://www.w3.org/ns/activitystreams",
			Type:         "Note",
			ID:           "https://" + fqdn + "/ap/note/" + msg.ID,
			AttributedTo: "https://" + fqdn + "/ap/acct/" + entity.ID,
			Content:      text,
			QuoteURL:     ref,
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
		}, nil
	} else {
		return Note{}, errors.New("invalid schema")
	}
}

