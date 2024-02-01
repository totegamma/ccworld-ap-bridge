package activitypub

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/totegamma/concurrent/x/core"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/util"
	"regexp"
	"strings"
	"time"
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
				Context: "https://www.w3.org/ns/activitystreams",
				Type:    "Announce",
				ID:      "https://" + fqdn + "/ap/note/" + msg.ID,
				Object:  ref,
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

func (h Handler) NoteToMessage(ctx context.Context, object Note, person Person, destStreams []string) (core.Message, error) {

	content := object.Content

	for _, attachment := range object.Attachment {
		if attachment.Type == "Document" {
			content += "\n\n![image](" + attachment.URL + ")"
		}
	}

	var emojis map[string]WorldEmoji = make(map[string]WorldEmoji)
	for _, tag := range object.Tag {
		if tag.Type == "Emoji" {
			name := strings.Trim(tag.Name, ":")
			emojis[name] = WorldEmoji{
				ImageURL: tag.Icon.URL,
			}
		}
	}

	if len(content) == 0 {
		return core.Message{}, errors.New("empty note")
	}

	if len(content) > 4096 {
		return core.Message{}, errors.New("note too long")
	}

	username := person.Name
	if len(username) == 0 {
		username = person.PreferredUsername
	}

	date, err := time.Parse(time.RFC3339Nano, object.Published)
	if err != nil {
		date = time.Now()
	}

	b := message.SignedObject{
		Signer: h.apconfig.ProxyCCID,
		Type:   "Message",
		Schema: "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/messages/note/0.0.1.json",
		Body: map[string]interface{}{
			"body": content,
			"profileOverride": map[string]interface{}{
				"username":    username,
				"avatar":      person.Icon.URL,
				"description": person.Summary,
				"link":        person.URL,
			},
			"emojis": emojis,
		},
		Meta: map[string]interface{}{
			"apActor":          person.URL,
			"apObjectRef":      object.ID,
			"apPublisherInbox": person.Inbox,
		},
		SignedAt: date,
	}

	objb, err := json.Marshal(b)
	if err != nil {
		return core.Message{}, err
	}

	objstr := string(objb)
	objsig, err := util.SignBytes(objb, h.apconfig.Proxy.PrivateKey)
	if err != nil {
		return core.Message{}, err
	}

	created, err := h.message.PostMessage(ctx, objstr, objsig, destStreams)
	if err != nil {
		return core.Message{}, err
	}

	return created, nil
}
