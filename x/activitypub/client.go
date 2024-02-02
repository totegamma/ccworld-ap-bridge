package activitypub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/totegamma/httpsig"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

var (
	UserAgent = "ConcurrentWorker/1.0"
)

// FetchNote fetches a note from remote ap server.
func (h Handler) FetchNote(ctx context.Context, noteID string, execEntity ApEntity) (Note, error) {
	_, span := tracer.Start(ctx, "FetchNote")
	defer span.End()

	var note Note
	req, err := http.NewRequest("GET", noteID, nil)
	if err != nil {
		return note, err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Host", req.URL.Host)
	client := new(http.Client)

	priv, err := h.repo.LoadKey(ctx, execEntity)
	if err != nil {
		log.Println(err)
		return note, err
	}

	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	digestAlgorithm := httpsig.DigestSha256
	headersToSign := []string{httpsig.RequestTarget, "date", "host"}
	signer, _, err := httpsig.NewSigner(prefs, digestAlgorithm, headersToSign, httpsig.Signature, 0)
	if err != nil {
		log.Println(err)
		return note, err
	}
	err = signer.SignRequest(priv, "https://"+h.config.Concurrent.FQDN+"/ap/acct/"+execEntity.ID+"#main-key", req, nil)

	resp, err := client.Do(req)
	if err != nil {
		return note, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return note, err
	}

	err = json.Unmarshal(body, &note)
	if err != nil {
		return note, err
	}

	return note, nil
}

// FetchPerson fetches a person from remote ap server.
func (h Handler) FetchPerson(ctx context.Context, actor string, execEntity ApEntity) (Person, error) {
	_, span := tracer.Start(ctx, "FetchPerson")
	defer span.End()

	// try cache
	cache, err := h.mc.Get(actor)
	if err == nil {
		person := Person{}
		err = json.Unmarshal(cache.Value, &person)
		if err == nil {
			return person, nil
		}
	}

	var person Person
	req, err := http.NewRequest("GET", actor, nil)
	if err != nil {
		return person, err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Host", req.URL.Host)
	client := new(http.Client)

	priv, err := h.repo.LoadKey(ctx, execEntity)
	if err != nil {
		log.Println(err)
		return person, err
	}

	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	digestAlgorithm := httpsig.DigestSha256
	headersToSign := []string{httpsig.RequestTarget, "date", "host"}
	signer, _, err := httpsig.NewSigner(prefs, digestAlgorithm, headersToSign, httpsig.Signature, 0)
	if err != nil {
		log.Println(err)
		return person, err
	}
	err = signer.SignRequest(priv, "https://"+h.config.Concurrent.FQDN+"/ap/acct/"+execEntity.ID+"#main-key", req, nil)

	resp, err := client.Do(req)
	if err != nil {
		return person, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	err = json.Unmarshal(body, &person)
	if err != nil {
		return person, err
	}

	// cache
	personBytes, err := json.Marshal(person)
	if err == nil {
		h.mc.Set(&memcache.Item{
			Key:        actor,
			Value:      personBytes,
			Expiration: 1800, // 30 minutes
		})
	}

	return person, nil
}

// ResolveActor resolves an actor from id notation.
func ResolveActor(ctx context.Context, id string) (string, error) {
	_, span := tracer.Start(ctx, "ResolveActor")
	defer span.End()

	if id[0] == '@' {
		id = id[1:]
	}

	split := strings.Split(id, "@")
	if len(split) != 2 {
		return "", fmt.Errorf("invalid id")
	}

	domain := split[1]

	targetlink := "https://" + domain + "/.well-known/webfinger?resource=acct:" + id

	var webfinger WebFinger
	req, err := http.NewRequest("GET", targetlink, nil)
	if err != nil {
		return "", err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Accept", "application/jrd+json")
	req.Header.Set("User-Agent", UserAgent)
	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	err = json.Unmarshal(body, &webfinger)
	if err != nil {
		fmt.Println(string(body))
		return "", err
	}

	var aplink WebFingerLink
	for _, link := range webfinger.Links {
		if link.Rel == "self" {
			aplink = link
		}
	}

	if aplink.Href == "" {
		return "", fmt.Errorf("no ap link found")
	}

	return aplink.Href, nil
}

// PostToInbox posts a message to remote ap server.
func (h Handler) PostToInbox(ctx context.Context, inbox string, object interface{}, entity ApEntity) error {

	objectBytes, err := json.Marshal(object)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", inbox, bytes.NewBuffer(objectBytes))
	if err != nil {
		return err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Host", req.URL.Host)
	client := new(http.Client)

	priv, err := h.repo.LoadKey(ctx, entity)
	if err != nil {
		log.Println(err)
		return err
	}

	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	digestAlgorithm := httpsig.DigestSha256
	headersToSign := []string{httpsig.RequestTarget, "date", "digest", "host"}
	signer, _, err := httpsig.NewSigner(prefs, digestAlgorithm, headersToSign, httpsig.Signature, 0)
	if err != nil {
		log.Println(err)
		return err
	}
	err = signer.SignRequest(priv, "https://"+h.config.Concurrent.FQDN+"/ap/acct/"+entity.ID+"#main-key", req, objectBytes)

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	log.Printf("POST %s [%d]: %s", inbox, resp.StatusCode, string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("error posting to inbox: %d", resp.StatusCode)
	}

	defer resp.Body.Close()

	return nil
}
