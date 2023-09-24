// Package activitypub provides an ActivityPub server.
package activitypub

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/util"
	"go.opentelemetry.io/otel"
	"golang.org/x/exp/slices"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var tracer = otel.Tracer("activitypub")

// Handler is a handler for the WebFinger protocol.
type Handler struct {
	repo        *Repository
	rdb         *redis.Client
	message     *message.Service
	association *association.Service
	config      util.Config
	apconfig    APConfig
}

// NewHandler returns a new Handler.
func NewHandler(
	repo *Repository,
	rdb *redis.Client,
	message *message.Service,
	association *association.Service,
	config util.Config,
	apconfig APConfig,
) *Handler {
	return &Handler{repo, rdb, message, association, config, apconfig}
}

// :: Activitypub Related Functions ::

// WebFinger handles WebFinger requests.
func (h Handler) WebFinger(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "WebFinger")
	defer span.End()

	resource := c.QueryParam("resource")
	split := strings.Split(resource, ":")
	if len(split) != 2 {
		return c.String(http.StatusBadRequest, "Invalid resource")
	}
	rt, id := split[0], split[1]
	if rt != "acct" {
		return c.String(http.StatusBadRequest, "Invalid resource")
	}
	split = strings.Split(id, "@")
	if len(split) != 2 {
		return c.String(http.StatusBadRequest, "Invalid resource")
	}
	username, domain := split[0], split[1]
	if domain != h.config.Concurrent.FQDN {
		return c.String(http.StatusBadRequest, "Invalid resource")
	}

	_, err := h.repo.GetEntityByID(ctx, username)
	if err != nil {
		return c.String(http.StatusNotFound, "entity not found")
	}

	return c.JSON(http.StatusOK, WebFinger{
		Subject: resource,
		Links: []WebFingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + username,
			},
		},
	})
}

// User handles user requests.
func (h Handler) User(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "User")
	defer span.End()

	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	entity, err := h.repo.GetEntityByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	person, err := h.repo.GetPersonByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "person not found")
	}

	// check if accept is application/activity+json or application/ld+json
	acceptHeader := c.Request().Header.Get("Accept")
	accept := strings.Split(acceptHeader, ",")

	if !slices.Contains(accept, "application/activity+json") && !slices.Contains(accept, "application/ld+json") {
		// redirect to user page
		return c.Redirect(http.StatusFound, "https://concurrent.world/entity/"+entity.CCID)
	}

	return c.JSON(http.StatusOK, Person{
		Context:           "https://www.w3.org/ns/activitystreams",
		Type:              "Person",
		ID:                "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id,
		Inbox:             "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id + "/inbox",
		Outbox:            "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id + "/outbox",
		PreferredUsername: id,
		Name:              person.Name,
		Summary:           person.Summary,
		URL:               "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id,
		Icon: Icon{
			Type:      "Image",
			MediaType: "image/png",
			URL:       person.IconURL,
		},
		PublicKey: Key{
			ID:           "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id + "#main-key",
			Type:         "Key",
			Owner:        "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id,
			PublicKeyPem: entity.Publickey,
		},
	})
}

// Note handles note requests.
func (h Handler) Note(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "Note")
	defer span.End()

	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "Invalid noteID")
	}

	msg, err := h.message.Get(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "message not found")
	}

	// check if accept is application/activity+json or application/ld+json
	acceptHeader := c.Request().Header.Get("Accept")
	accept := strings.Split(acceptHeader, ",")

	if !slices.Contains(accept, "application/activity+json") && !slices.Contains(accept, "application/ld+json") {
		// redirect to user page
		return c.Redirect(http.StatusFound, "https://concurrent.world/message/"+id+"@"+msg.Author)
	}

	note, err := h.MessageToNote(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "error converting message to note")
	}

	return c.JSON(http.StatusOK, note)
}

// Inbox handles inbox requests.
func (h Handler) Inbox(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "HandlerAPInbox")
	defer span.End()

	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	_, err := h.repo.GetEntityByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	var object Object
	err = c.Bind(&object)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	switch object.Type {
	case "Follow":

		requester, err := FetchPerson(ctx, object.Actor)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		accept := Accept{
			Context: "https://www.w3.org/ns/activitystreams",
			ID:      "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id + "/follows/" + url.PathEscape(requester.ID),
			Type:    "Accept",
			Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + id,
			Object:  object,
		}

		split := strings.Split(object.Object.(string), "/")
		userID := split[len(split)-1]

		err = h.PostToInbox(ctx, requester.Inbox, accept, userID)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error")
		}

		// check follow already exists
		_, err = h.repo.GetFollowerByID(ctx, object.ID)
		if err == nil {
			return c.String(http.StatusOK, "follow already exists")
		}

		// dump object
		b, err := json.Marshal(object)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error (object dump error)")
		}
		log.Println(string(b))

		// save follow
		err = h.repo.SaveFollower(ctx, ApFollower{
			ID:                  object.ID,
			SubscriberInbox:     requester.Inbox,
			SubscriberPersonURL: requester.ID,
			PublisherUserID:     userID,
		})
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error (save follow error)")
		}

		return c.String(http.StatusOK, "follow accepted")

	case "Like":
		targetID := strings.Replace(object.Object.(string), "https://"+h.config.Concurrent.FQDN+"/ap/note/", "", 1)
		_, err := h.message.Get(ctx, targetID)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "message not found")
		}

		person, err := FetchPerson(ctx, object.Actor)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "failed to fetch actor")
		}

		var obj association.SignedObject

		if (object.Tag == nil) || (object.Tag[0].Name[0] != ':') {
			obj = association.SignedObject{
				Signer: h.apconfig.ProxyCCID,
				Type:   "Association",
				Schema: "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/like/0.0.1.json",
				Body: map[string]interface{}{
					"profileOverride": map[string]interface{}{
						"username":    person.Name,
						"avatar":      person.Icon.URL,
						"description": person.Summary,
						"link":        object.Actor,
					},
				},
				Meta: map[string]interface{}{
					"apActor": object.Actor,
				},
				SignedAt: time.Now(),
				Target:   targetID,
			}
		} else {
			obj = association.SignedObject{
				Signer: h.apconfig.ProxyCCID,
				Type:   "Association",
				Schema: "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/associations/emoji/0.0.1.json",
				Body: map[string]interface{}{
					"shortcode": object.Tag[0].Name,
					"imageUrl":  object.Tag[0].Icon.URL,
					"profileOverride": map[string]interface{}{
						"username":    person.Name,
						"avatar":      person.Icon.URL,
						"description": person.Summary,
						"link":        object.Actor,
					},
				},
				Meta: map[string]interface{}{
					"apActor": object.Actor,
				},
				SignedAt: time.Now(),
				Target:   targetID,
			}
		}

		objb, err := json.Marshal(obj)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "Internal server error (json marshal error)")
		}

		objstr := string(objb)
		objsig, err := util.SignBytes(objb, h.apconfig.Proxy.PrivateKey)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "Internal server error (sign error)")
		}

		_, err = h.association.PostAssociation(ctx, objstr, objsig, []string{}, "messages")
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "Internal server error (post association error)")
		}

		return c.String(http.StatusOK, "like accepted")

	case "Create":
		createObject, ok := object.Object.(map[string]interface{})
		if !ok {
			log.Println("Invalid create object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		createType, ok := createObject["type"].(string)
		if !ok {
			log.Println("Invalid create object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		createID, ok := createObject["id"].(string)
		if !ok {
			log.Println("Invalid create object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		switch createType {
		case "Note":
			// check if the note is already exists
			_, err := h.repo.GetApObjectCrossReferenceByCcObjectID(ctx, createID)
			if err == nil {
				// already exists
				return c.String(http.StatusOK, "note already exists")
			}

			// list up follows
			follows, err := h.repo.GetFollowsByPublisher(ctx, object.Actor)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (get follows error)")
			}

			destStreams := []string{}
			for _, follow := range follows {
				entity, err := h.repo.GetEntityByID(ctx, follow.SubscriberUserID)
				if err != nil {
					span.RecordError(err)
					continue
				}
				destStreams = append(destStreams, entity.FollowStream)
			}

			person, err := FetchPerson(ctx, object.Actor)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusBadRequest, "failed to fetch actor")
			}

			content, ok := createObject["content"].(string)
			if !ok {
				log.Println("Invalid create object", object.Object)
				return c.String(http.StatusBadRequest, "Invalid request body")
			}


			attachments, ok := createObject["attachment"].([]interface{})
			if ok {
				for _, attachment := range attachments {
					attachment, ok := attachment.(map[string]interface{})
					if !ok {
						continue
					}
					atype, ok := attachment["type"].(string)
					if !ok {
						continue
					}
					aurl, ok := attachment["url"].(string)
					if !ok {
						continue
					}
					if atype == "Document" {
						content += "\n\n![image](" + aurl + ")"
					}
				}
			}

			var emojis map[string]WorldEmoji = make(map[string]WorldEmoji)
			tags, ok := createObject["tag"].([]interface{})
			if ok {
				for _, tag := range tags {
					tag, ok := tag.(map[string]interface{})
					if !ok {
						continue
					}
					tagtype, ok := tag["type"].(string)
					if !ok {
						continue
					}
					if tagtype == "Emoji" {
						name, ok := tag["name"].(string)
						if !ok {
							continue
						}
						name = strings.Trim(name, ":")
						icon, ok := tag["icon"].(map[string]interface{})
						if !ok {
							continue
						}
						url, ok := icon["url"].(string)
						if !ok {
							continue
						}
						emojis[name] = WorldEmoji{
							ImageURL: url,
						}
					}
				}
			}

			if len(content) == 0 {
				return c.String(http.StatusOK, "empty note")
			}

			if len(content) > 4096 {
				return c.String(http.StatusOK, "note too long")
			}

			b := message.SignedObject{
				Signer: h.apconfig.ProxyCCID,
				Type:   "Message",
				Schema: "https://raw.githubusercontent.com/totegamma/concurrent-schemas/master/messages/note/0.0.1.json",
				Body: map[string]interface{}{
					"body": content,
					"profileOverride": map[string]interface{}{
						"username":    person.Name,
						"avatar":      person.Icon.URL,
						"description": person.Summary,
						"link":        object.Actor,
					},
					"emojis": emojis,
				},
				Meta: map[string]interface{}{
					"apActor":          object.Actor,
					"apObjectRef":      createID,
					"apPublisherInbox": person.Inbox,
				},
			}

			objb, err := json.Marshal(b)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
			}

			objstr := string(objb)
			objsig, err := util.SignBytes(objb, h.apconfig.Proxy.PrivateKey)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (sign error)")
			}

			created, err := h.message.PostMessage(ctx, objstr, objsig, destStreams)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (post message error)")
			}

			// save cross reference
			err = h.repo.CreateApObjectCrossReference(ctx, ApObjectCrossReference{
				ApObjectID:   createID,
				CcObjectID:   created.ID,
			})

			return c.String(http.StatusOK, "note accepted")
		default:
			// print request body
			b, err := json.Marshal(object)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
			}
			log.Println("Unhandled Create Object", string(b))
			return c.String(http.StatusOK, "OK but not implemented")
		}

	case "Accept":
		acceptObject, ok := object.Object.(map[string]interface{})
		if !ok {
			log.Println("Invalid accept object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		acceptType, ok := acceptObject["type"].(string)
		if !ok {
			log.Println("Invalid accept object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		switch acceptType {
		case "Follow":
			objectID, ok := acceptObject["id"].(string)
			if !ok {
				log.Println("Invalid accept object", object.Object)
				return c.String(http.StatusBadRequest, "Invalid request body")
			}
			apFollow, err := h.repo.GetFollowByID(ctx, objectID)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusNotFound, "follow not found")
			}
			apFollow.Accepted = true

			_, err = h.repo.UpdateFollow(ctx, apFollow)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (update follow error)")
			}

			return c.String(http.StatusOK, "follow accepted")
		default:
			// print request body
			b, err := json.Marshal(object)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
			}
			log.Println("Unhandled accept object", string(b))
			return c.String(http.StatusOK, "OK but not implemented")

		}

	case "Undo":
		undoObject, ok := object.Object.(map[string]interface{})
		if !ok {
			log.Println("Invalid undo object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		undoType, ok := undoObject["type"].(string)
		if !ok {
			log.Println("Invalid undo object", object.Object)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		switch undoType {
		case "Follow":

			remote, ok := undoObject["actor"].(string)
			if !ok {
				log.Println("Invalid undo object", object.Object)
				return c.String(http.StatusBadRequest, "Invalid request body")
			}

			obj, ok := undoObject["object"].(string)
			if !ok {
				log.Println("Invalid undo object", object.Object)
				return c.String(http.StatusBadRequest, "Invalid request body")
			}

			local := strings.TrimPrefix(obj, "https://"+h.config.Concurrent.FQDN+"/ap/acct/")

			// check follow already deleted
			_, err = h.repo.GetFollowerByTuple(ctx, local, remote)
			if err != nil {
				return c.String(http.StatusOK, "follow already undoed")
			}
			h.repo.RemoveFollower(ctx, local, remote)
			return c.String(http.StatusOK, "OK")
		default:
			// print request body
			b, err := json.Marshal(object)
			if err != nil {
				span.RecordError(err)
				return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
			}
			log.Println("Unhandled Undo Object", string(b))
			return c.String(http.StatusOK, "OK but not implemented")
		}
	case "Delete":
		deleteObject, ok := object.Object.(map[string]interface{})
		if !ok {
			log.Println("Invalid delete object", object.Object)
			return c.String(http.StatusOK, "Invalid request body")
		}
		deleteID, ok := deleteObject["id"].(string)
		if !ok {
			log.Println("Invalid delete object", object.Object)
			return c.String(http.StatusOK, "Invalid request body")
		}

		deleteRef, err := h.repo.GetApObjectCrossReferenceByApObjectID(ctx, deleteID)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusOK, "Object Already Deleted")
		}

		_, err = h.message.Delete(ctx, deleteRef.CcObjectID)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error (delete error)")
		}

		err = h.repo.DeleteApObjectCrossReference(ctx, deleteRef)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error (delete error)")
		}
		return c.String(http.StatusOK, "Deleted")

	default:
		// print request body
		b, err := json.Marshal(object)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
		}
		log.Println("Unhandled Activitypub Object", string(b))
		return c.String(http.StatusOK, "OK but not implemented")
	}

	// return c.String(http.StatusInternalServerError, "Internal server error")
}

// :: Database related functions ::

// GetPerson handles entity fetches.
func (h Handler) GetPerson(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "GetPerson")
	defer span.End()

	id := c.Param("id")
	if id == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	person, err := h.repo.GetPersonByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": person})
}

// UpdatePerson handles entity updates.
func (h Handler) UpdatePerson(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "UpdatePerson")
	defer span.End()

	claims := c.Get("jwtclaims").(util.JwtClaims)
	ccid := claims.Audience

	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	if entity.CCID != ccid {
		return c.String(http.StatusUnauthorized, "unauthorized")
	}

	var person ApPerson
	err = c.Bind(&person)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	created, err := h.repo.UpsertPerson(ctx, person)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": created})
}

// Follow handles entity follow requests.
func (h Handler) Follow(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "Follow")
	defer span.End()

	claims := c.Get("jwtclaims").(util.JwtClaims)
	ccid := claims.Audience
	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	targetID := c.Param("id")
	if targetID == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	if targetID[0] != '@' {
		targetID = "@" + targetID
	}

	log.Println("follow", targetID)

	targetActor, err := ResolveActor(ctx, targetID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	targetPerson, err := FetchPerson(ctx, targetActor)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	simpleID := strings.Replace(targetID, "@", "-", -1)
	simpleID = strings.Replace(simpleID, ".", "-", -1)
	followID := "https://" + h.config.Concurrent.FQDN + "/follow/" + entity.ID + "/" + simpleID

	followObject := Object{
		Context: "https://www.w3.org/ns/activitystreams",
		Type:    "Follow",
		Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + entity.ID,
		Object:  targetPerson.ID,
		ID:      followID,
	}

	// debug: print follow object
	b, err := json.Marshal(followObject)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error (json marshal error)")
	}
	log.Println("Follow Object", string(b))

	err = h.PostToInbox(ctx, targetPerson.Inbox, followObject, entity.ID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}

	follow := ApFollow{
		ID:                 followID,
		PublisherPersonURL: targetPerson.ID,
		SubscriberUserID:   entity.ID,
	}

	err = h.repo.SaveFollow(ctx, follow)

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": follow})
}

// Unfollow handles entity unfollow requests.
func (h Handler) UnFollow(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "Unfollow")
	defer span.End()

	claims := c.Get("jwtclaims").(util.JwtClaims)
	ccid := claims.Audience

	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	targetID := c.Param("id")
	if targetID == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	if targetID[0] != '@' {
		targetID = "@" + targetID
	}

	simpleID := strings.Replace(targetID, "@", "-", -1)
	simpleID = strings.Replace(simpleID, ".", "-", -1)
	followID := "https://" + h.config.Concurrent.FQDN + "/follow/" + entity.ID + "/" + simpleID
	log.Println("unfollow", followID)

	targetActor, err := ResolveActor(ctx, targetID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	targetPerson, err := FetchPerson(ctx, targetActor)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	undoObject := Object{
		Context: "https://www.w3.org/ns/activitystreams",
		Type:    "Undo",
		Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + entity.ID,
		ID:      followID + "/undo",
		Object: Object{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Follow",
			ID:      followID,
			Actor:   "https://" + h.config.Concurrent.FQDN + "/ap/acct/" + entity.ID,
			Object:  targetPerson.ID,
		},
	}

	// dump undo object
	undoJSON, err := json.Marshal(undoObject)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}
	log.Println(string(undoJSON))

	err = h.PostToInbox(ctx, targetPerson.Inbox, undoObject, entity.ID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}

	deleted, err := h.repo.RemoveFollow(ctx, followID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": deleted})
}

// CreateEntity handles entity creation.
func (h Handler) CreateEntity(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "CreateEntity")
	defer span.End()

	claims := c.Get("jwtclaims").(util.JwtClaims)
	ccid := claims.Audience

	var request CreateEntityRequest
	err := c.Bind(&request)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	// check if entity already exists
	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err == nil { // Update
		entity.HomeStream = request.HomeStream
		entity.NotificationStream = request.NotificationStream
		entity.FollowStream = request.FollowStream

		updated, err := h.repo.UpdateEntity(ctx, entity)
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error")
		}

		updated.Privatekey = ""

		return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": updated})
	} else { // Create

		// RSAキーペアの生成
		privKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}

		// 秘密鍵をPEM形式に変換
		privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
		privKeyPEM := pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: privKeyBytes,
			},
		)

		// 公開鍵をPEM形式に変換
		pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
		if err != nil {
			panic(err)
		}
		pubKeyPEM := pem.EncodeToMemory(
			&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: pubKeyBytes,
			},
		)

		created, err := h.repo.CreateEntity(ctx, ApEntity{
			ID:                 request.ID,
			CCID:               ccid,
			Publickey:          string(pubKeyPEM),
			Privatekey:         string(privKeyPEM),
			HomeStream:         request.HomeStream,
			NotificationStream: request.NotificationStream,
			FollowStream:       request.FollowStream,
		})
		if err != nil {
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "Internal server error")
		}

		created.Privatekey = ""

		return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": created})
	}
}

// GetEntityID handles entity id requests.
func (h Handler) GetEntityID(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "GetEntityID")
	defer span.End()

	ccid := c.Param("ccid")
	if ccid == "" {
		return c.String(http.StatusBadRequest, "Invalid username")
	}

	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	entity.Privatekey = ""

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": entity})
}

func (h Handler) NodeInfoWellKnown(c echo.Context) error {
	_, span := tracer.Start(c.Request().Context(), "NodeInfoWellKnown")
	defer span.End()

	return c.JSON(http.StatusOK, WellKnown{
		Links: []WellKnownLink{
			{
				Rel:  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				Href: "https://" + h.config.Concurrent.FQDN + "/ap/nodeinfo/2.0",
			},
		},
	})
}

// GetStats handles stats requests
func (h Handler) GetStats(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "GetStats")
	defer span.End()

	claims := c.Get("jwtclaims").(util.JwtClaims)
	ccid := claims.Audience

	entity, err := h.repo.GetEntityByCCID(ctx, ccid)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusNotFound, "entity not found")
	}

	follows := make([]string, 0)
	apFollows, err := h.repo.GetFollows(ctx, entity.ID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}
	for _, f := range apFollows {
		follows = append(follows, f.PublisherPersonURL)
	}

	followers := make([]string, 0)
	apFollowers, err := h.repo.GetFollowers(ctx, entity.ID)
	if err != nil {
		span.RecordError(err)
		return c.String(http.StatusInternalServerError, "Internal server error")
	}
	for _, f := range apFollowers {
		followers = append(followers, f.SubscriberPersonURL)
	}

	stats := ApAccountStats{
		Follows:   follows,
		Followers: followers,
	}

	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "content": stats})
}

// NodeInfo handles nodeinfo requests
func (h Handler) NodeInfo(c echo.Context) error {
	_, span := tracer.Start(c.Request().Context(), "NodeInfo")
	defer span.End()

	return c.JSON(http.StatusOK, NodeInfo{
		Version: "2.0",
		Software: NodeInfoSoftware{
			Name:    "Concurrent",
			Version: util.GetGitShortHash(),
		},
		Protocols: []string{
			"activitypub",
		},
		OpenRegistrations: h.config.Concurrent.Registration == "open",
		Metadata: NodeInfoMetadata{
			NodeName:        h.config.Profile.Nickname,
			NodeDescription: h.config.Profile.Description,
			Maintainer: NodeInfoMetadataMaintainer{
				Name:  h.config.Profile.MaintainerName,
				Email: h.config.Profile.MaintainerEmail,
			},
			ThemeColor: h.config.Profile.ThemeColor,
		},
	})
}

// PrintRequest prints the request body.
func (h Handler) PrintRequest(c echo.Context) error {

	body := c.Request().Body
	bytes, err := ioutil.ReadAll(body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Internal server error")
	}
	fmt.Println(string(bytes))

	return c.String(http.StatusOK, "ok")
}
