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

// ApObjectReference is a db model of an ActivityPub object cross reference.
type ApObjectReference struct {
	ApObjectID string `json:"apobjectID" gorm:"primaryKey;type:text;"`
	CcObjectID string `json:"ccobjectID" gorm:"type:text;"`
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

// endpoints is a struct for the endpoints field of a WebFinger response.
type PersonEndpoints struct {
	SharedInbox string `json:"sharedInbox,omitempty"`
}

// Person is a struct for an ActivityPub actor.
type Person struct {
	Context           interface{}     `json:"@context,omitempty"`
	Type              string          `json:"type,omitempty"`
	ID                string          `json:"id,omitempty"`
	Inbox             string          `json:"inbox,omitempty"`
	Outbox            string          `json:"outbox,omitempty"`
	SharedInbox       string          `json:"sharedInbox,omitempty"`
	Endpoints         PersonEndpoints `json:"endpoints,omitempty"`
	Followers         string          `json:"followers,omitempty"`
	Following         string          `json:"following,omitempty"`
	Liked             string          `json:"liked,omitempty"`
	PreferredUsername string          `json:"preferredUsername,omitempty"`
	Name              string          `json:"name,omitempty"`
	Summary           string          `json:"summary,omitempty"`
	URL               string          `json:"url,omitempty"`
	Icon              Icon            `json:"icon,omitempty"`
	PublicKey         Key             `json:"publicKey,omitempty"`
}

// Key is a struct for the publicKey field of an actor.
type Key struct {
	ID           string `json:"id,omitempty"`
	Type         string `json:"type,omitempty"`
	Owner        string `json:"owner,omitempty"`
	PublicKeyPem string `json:"publicKeyPem,omitempty"`
}

// Icon is a struct for the icon field of an actor.
type Icon struct {
	Type      string `json:"type,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Create is a struct for an ActivityPub create activity.
type Create struct {
	Context interface{} `json:"@context,omitempty"`
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type,omitempty"`
	Actor   string      `json:"actor,omitempty"`
	To      []string    `json:"to,omitempty"`
	Object  interface{} `json:"object,omitempty"`
}

// Object is a struct for an ActivityPub object.
type Object struct {
	Context    interface{}  `json:"@context,omitempty"`
	Type       string       `json:"type,omitempty"`
	ID         string       `json:"id,omitempty"`
	Content    string       `json:"content,omitempty"`
	Actor      string       `json:"actor,omitempty"`
	Object     interface{}  `json:"object,omitempty"`
	To         []string     `json:"to,omitempty"`
	Attachment []Attachment `json:"attachment,omitempty"`
	Tag        []Tag        `json:"tag,omitempty"`
}

// Attachment is a struct for an ActivityPub attachment.
type Attachment struct {
	Type      string `json:"type,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Tag is a struct for an ActivityPub tag.
type Tag struct {
	Type string `json:"type,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Icon Icon   `json:"icon,omitempty"`
}

// Accept is a struct for an ActivityPub accept activity.
type Accept struct {
	Context interface{} `json:"@context,omitempty"`
	Type    string      `json:"type,omitempty"`
	ID      string      `json:"id,omitempty"`
	Actor   string      `json:"actor,omitempty"`
	Object  Object      `json:"object,omitempty"`
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
	Context      interface{}  `json:"@context,omitempty"`
	Type         string       `json:"type,omitempty"`
	ID           string       `json:"id,omitempty"`
	AttributedTo string       `json:"attributedTo,omitempty"`
	InReplyTo    string       `json:"inReplyTo,omitempty"`
	QuoteURL     string       `json:"quoteUrl,omitempty"`
	Content      string       `json:"content,omitempty"`
	Published    string       `json:"published,omitempty"`
	To           []string     `json:"to,omitempty"`
	Tag          []Tag        `json:"tag,omitempty"`
	Attachment   []Attachment `json:"attachment,omitempty"`
	Object       interface{}  `json:"object,omitempty"`
	Sensitive    bool         `json:"sensitive,omitempty"`
	Summary      string       `json:"summary,omitempty"`
}

type NodeInfoUsers struct {
	TotalUsers int64 `json:"total,omitempty"`
}

type NodeInfoUsage struct {
	LocalPosts int64         `json:"localPosts,omitempty"`
	Users      NodeInfoUsers `json:"users,omitempty"`
}

// NodeInfo is a struct for a NodeInfo response.
type NodeInfo struct {
	Version           string           `json:"version,omitempty"`
	Software          NodeInfoSoftware `json:"software,omitempty"`
	Protocols         []string         `json:"protocols,omitempty"`
	OpenRegistrations bool             `json:"openRegistrations,omitempty"`
	Metadata          NodeInfoMetadata `json:"metadata,omitempty"`
	Usage             NodeInfoUsage    `json:"usage,omitempty"`
}

// NodeInfoSoftware is a struct for the software field of a NodeInfo response.
type NodeInfoSoftware struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// NodeInfoMetadata is a struct for the metadata field of a NodeInfo response.
type NodeInfoMetadata struct {
	NodeName        string                     `json:"nodeName,omitempty"`
	NodeDescription string                     `json:"nodeDescription,omitempty"`
	Maintainer      NodeInfoMetadataMaintainer `json:"maintainer,omitempty"`
	ThemeColor      string                     `json:"themeColor,omitempty"`
}

// NodeInfoMetadataMaintainer is a struct for the maintainer field of a NodeInfo response.
type NodeInfoMetadataMaintainer struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
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
