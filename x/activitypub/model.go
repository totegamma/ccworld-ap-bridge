package activitypub

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-yaml/yaml"
	"log"
	"os"
)

// ApEntity is a db model of an ActivityPub entity.
type ApEntity struct {
	ID                 string `json:"id" gorm:"type:text"`
	CCID               string `json:"ccid" gorm:"type:char(42)"`
	Publickey          string `json:"publickey" gorm:"type:text"`
	Privatekey         string `json:"privatekey" gorm:"type:text"`
	HomeStream         string `json:"homestream" gorm:"type:text"`
	NotificationStream string `json:"notificationstream" gorm:"type:text"`
	FollowStream       string `json:"followstream" gorm:"type:text"`
}

// ApPerson is a db model of an ActivityPub entity.
type ApPerson struct {
	ID      string `json:"id" gorm:"type:text"`
	Name    string `json:"name" gorm:"type:text"`
	Summary string `json:"summary" gorm:"type:text"`
	IconURL string `json:"icon_url" gorm:"type:text"`
}

// ApFollow is a db model of an ActivityPub follow.
// Concurrent -> Activitypub
type ApFollow struct {
	ID                 string `json:"id" gorm:"type:text"`
	Accepted           bool   `json:"accepted" gorm:"type:bool"`
	PublisherPersonURL string `json:"publisher" gorm:"type:text"`  // ActivityPub Person
	SubscriberUserID   string `json:"subscriber" gorm:"type:text"` // Concurrent APID
}

// ApFollwer is a db model of an ActivityPub follower.
// Activitypub -> Concurrent
type ApFollower struct {
	ID                  string `json:"id" gorm:"type:text"`
	SubscriberPersonURL string `json:"subscriber" gorm:"type:text;uniqueIndex:uniq_apfollower;"` // ActivityPub Person
	PublisherUserID     string `json:"publisher" gorm:"type:text;uniqueIndex:uniq_apfollower;"`  // Concurrent APID
	SubscriberInbox     string `json:"subscriber_inbox" gorm:"type:text"`                        // ActivityPub Inbox
}

// WellKnown is a struct for a well-known response.
type WellKnown struct {
	// Subject string `json:"subject"`
	Links []WellKnownLink `json:"links"`
}

// WellKnownLink is a struct for the links field of a well-known response.
type WellKnownLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// WebFinger is a struct for a WebFinger response.
type WebFinger struct {
	Subject string          `json:"subject"`
	Links   []WebFingerLink `json:"links"`
}

// WebFingerLink is a struct for the links field of a WebFinger response.
type WebFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href"`
}

// Person is a struct for an ActivityPub actor.
type Person struct {
	Context           interface{} `json:"@context"`
	Type              string      `json:"type"`
	ID                string      `json:"id"`
	Inbox             string      `json:"inbox"`
	Outbox            string      `json:"outbox"`
	Followers         string      `json:"followers"`
	Following         string      `json:"following"`
	Liked             string      `json:"liked"`
	PreferredUsername string      `json:"preferredUsername"`
	Name              string      `json:"name"`
	Summary           string      `json:"summary"`
	URL               string      `json:"url"`
	Icon              Icon        `json:"icon"`
	PublicKey         Key         `json:"publicKey"`
}

// Key is a struct for the publicKey field of an actor.
type Key struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

// Icon is a struct for the icon field of an actor.
type Icon struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
}

// Create is a struct for an ActivityPub create activity.
type Create struct {
	Context interface{} `json:"@context"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Actor   string      `json:"actor"`
	To      []string    `json:"to"`
	Object  interface{} `json:"object"`
}

// Object is a struct for an ActivityPub object.
type Object struct {
	Context    interface{}  `json:"@context"`
	Type       string       `json:"type"`
	ID         string       `json:"id"`
	Content    string       `json:"content"`
	Actor      string       `json:"actor"`
	Object     interface{}  `json:"object"`
	To         []string     `json:"to"`
	Attachment []Attachment `json:"attachment"`
	Tag        []Tag        `json:"tag"`
}

// Attachment is a struct for an ActivityPub attachment.
type Attachment struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
}

// Tag is a struct for an ActivityPub tag.
type Tag struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon Icon   `json:"icon"`
}

// Accept is a struct for an ActivityPub accept activity.
type Accept struct {
	Context interface{} `json:"@context"`
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	Actor   string      `json:"actor"`
	Object  Object      `json:"object"`
}

// CreateEntityRequest is a struct for a request to create an entity.
type CreateEntityRequest struct {
	ID                 string `json:"id"`
	HomeStream         string `json:"homestream" gorm:"type:text"`
	NotificationStream string `json:"notificationstream" gorm:"type:text"`
	FollowStream       string `json:"followstream" gorm:"type:text"`
}

type ApAccountStats struct {
	Follows   []string `json:"follows"`
	Followers []string `json:"followers"`
}

// Note is a struct for a note.
type Note struct {
	Context      interface{}  `json:"@context"`
	Type         string       `json:"type"`
	ID           string       `json:"id"`
	AttributedTo string       `json:"attributedTo"`
	InReplyTo    string       `json:"inReplyTo"`
	QuoteURL     string       `json:"quoteUrl"`
	Content      string       `json:"content"`
	Published    string       `json:"published"`
	To           []string     `json:"to"`
	Tag          []Tag        `json:"tag"`
	Attachment   []Attachment `json:"attachment"`
	Object       interface{}  `json:"object"`
}

// NodeInfo is a struct for a NodeInfo response.
type NodeInfo struct {
	Version           string           `json:"version"`
	Software          NodeInfoSoftware `json:"software"`
	Protocols         []string         `json:"protocols"`
	OpenRegistrations bool             `json:"openRegistrations"`
	Metadata          NodeInfoMetadata `json:"metadata"`
}

// NodeInfoSoftware is a struct for the software field of a NodeInfo response.
type NodeInfoSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// NodeInfoMetadata is a struct for the metadata field of a NodeInfo response.
type NodeInfoMetadata struct {
	NodeName        string                     `json:"nodeName"`
	NodeDescription string                     `json:"nodeDescription"`
	Maintainer      NodeInfoMetadataMaintainer `json:"maintainer"`
	ThemeColor      string                     `json:"themeColor"`
}

// NodeInfoMetadataMaintainer is a struct for the maintainer field of a NodeInfo response.
type NodeInfoMetadataMaintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type WorldEmoji struct {
	ImageURL string `json:"imageURL"`
}

type ProxySettings struct {
	PrivateKey         string `yaml:"privateKey"`
	NotificationStream string `yaml:"notificationStream"`
}

type APConfig struct {
	Proxy ProxySettings `yaml:"proxy"`

	// internal generated
	ProxyCCID      string
	ProxyPublicKey string
}

// Load loads concurrent config from given path
func (c *APConfig) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal("failed to open configuration file:", err)
		return err
	}
	defer f.Close()

	err = yaml.NewDecoder(f).Decode(&c)
	if err != nil {
		log.Fatal("failed to load configuration file:", err)
		return err
	}

	// generate worker public key
	proxyPrivateKey, err := crypto.HexToECDSA(c.Proxy.PrivateKey)
	if err != nil {
		log.Fatal("failed to parse worker private key:", err)
		return err
	}
	c.ProxyPublicKey = hex.EncodeToString(crypto.FromECDSAPub(&proxyPrivateKey.PublicKey))

	// generate worker WorkerCCID
	addr := crypto.PubkeyToAddress(proxyPrivateKey.PublicKey)
	c.ProxyCCID = "CC" + addr.Hex()[2:]

	return nil
}
