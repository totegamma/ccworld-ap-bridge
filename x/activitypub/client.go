package activitypub

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/go-fed/httpsig"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	UserAgent = "ConcurrentWorker/1.0"
)

// FetchPerson fetches a person from remote ap server.
func FetchPerson(ctx context.Context, actor string) (Person, error) {
	_, span := tracer.Start(ctx, "FetchPerson")
	defer span.End()

	var person Person
	req, err := http.NewRequest("GET", actor, nil)
	if err != nil {
		return person, err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", UserAgent)
	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return person, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &person)
	if err != nil {
		return person, err
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

	targetlink := "https://"+domain+"/.well-known/webfinger?resource=acct:"+id

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

	body, _ := ioutil.ReadAll(resp.Body)

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
func (h Handler) PostToInbox(ctx context.Context, inbox string, object interface{}, signUser string) error {

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
	client := new(http.Client)

	entity, err := h.repo.GetEntityByID(ctx, signUser)
	if err != nil {
		return err
	}

	block, _ := pem.Decode([]byte(entity.Privatekey))
	if block == nil {
		return fmt.Errorf("failed to parse PEM block containing the key")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse DER encoded private key: " + err.Error())
	}

	prefs := []httpsig.Algorithm{httpsig.RSA_SHA256}
	digestAlgorithm := httpsig.DigestSha256
	headersToSign := []string{httpsig.RequestTarget, "date", "digest"}
	signer, _, err := httpsig.NewSigner(prefs, digestAlgorithm, headersToSign, httpsig.Signature, 0)
	if err != nil {
		log.Println(err)
		return err
	}
	err = signer.SignRequest(priv, "https://"+h.config.Concurrent.FQDN+"/ap/acct/"+signUser+"#main-key", req, objectBytes)

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
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
