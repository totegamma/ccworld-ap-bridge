package activitypub

import (
	"fmt"
	"crypto/x509"
	"encoding/pem"
	"context"
	"crypto/rsa"
	"gorm.io/gorm"
)


// Repository is a repository for ActivityPub.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a new Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetEntityByID returns an entity by ID.
func (r Repository) GetEntityByID(ctx context.Context, id string) (ApEntity, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetEntityByID")
	defer span.End()

	var entity ApEntity
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&entity)
	return entity, result.Error
}

// GetEntityByCCID returns an entity by CCiD.
func (r Repository) GetEntityByCCID(ctx context.Context, ccid string) (ApEntity, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetEntityByCCID")
	defer span.End()

	var entity ApEntity
	result := r.db.WithContext(ctx).Where("cc_id = ?", ccid).First(&entity)
	return entity, result.Error
}

// CreateEntity creates an entity.
func (r Repository) CreateEntity(ctx context.Context, entity ApEntity) (ApEntity, error) {
	ctx, span := tracer.Start(ctx, "RepositoryCreateEntity")
	defer span.End()

	result := r.db.WithContext(ctx).Create(&entity)
	return entity, result.Error
}

// UpdateEntity updates an entity.
func (r Repository) UpdateEntity(ctx context.Context, entity ApEntity) (ApEntity, error) {
	ctx, span := tracer.Start(ctx, "RepositoryUpdateEntity")
	defer span.End()

	result := r.db.WithContext(ctx).Save(&entity)
	return entity, result.Error
}

// GetPersonByID returns a person by ID.
func (r Repository) GetPersonByID(ctx context.Context, id string) (ApPerson, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetPersonByID")
	defer span.End()

	var person ApPerson
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&person)
	return person, result.Error
}

// UpsertPerson upserts a person.
func (r Repository) UpsertPerson(ctx context.Context, person ApPerson) (ApPerson, error) {
	ctx, span := tracer.Start(ctx, "RepositoryUpsertPerson")
	defer span.End()

	result := r.db.WithContext(ctx).Save(&person)
	return person, result.Error
}

// Save Follower action
func (r *Repository) SaveFollower(ctx context.Context, follower ApFollower) error {
	ctx, span := tracer.Start(ctx, "RepositorySaveFollow")
	defer span.End()

	return r.db.WithContext(ctx).Create(&follower).Error
}

// SaveFollowing saves follow action
func (r *Repository) SaveFollow(ctx context.Context, follow ApFollow) error {
	ctx, span := tracer.Start(ctx, "RepositorySaveFollow")
	defer span.End()

	return r.db.WithContext(ctx).Create(&follow).Error
}

// GetFollows returns owners follows
func (r *Repository) GetFollows(ctx context.Context, ownerID string) ([]ApFollow, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollows")
	defer span.End()

	var follows []ApFollow
	err := r.db.WithContext(ctx).Where("subscriber_user_id= ?", ownerID).Find(&follows).Error
	return follows, err
}

// GetFollowers returns owners followers
func (r *Repository) GetFollowers(ctx context.Context, ownerID string) ([]ApFollower, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollowers")
	defer span.End()

	var followers []ApFollower
	err := r.db.WithContext(ctx).Where("publisher_user_id= ?", ownerID).Find(&followers).Error
	return followers, err
}

// GetFollowsByPublisher returns follows by publisher
func (r *Repository) GetFollowsByPublisher(ctx context.Context, publisher string) ([]ApFollow, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollowsByPublisher")
	defer span.End()

	var follows []ApFollow
	err := r.db.WithContext(ctx).Where("publisher_person_url = ?", publisher).Find(&follows).Error
	return follows, err
}

// GetFollowerByTuple returns follow by tuple
func (r *Repository) GetFollowerByTuple(ctx context.Context, local, remote string) (ApFollower, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollowerByTuple")
	defer span.End()

	var follower ApFollower
	result := r.db.WithContext(ctx).Where("publisher_user_id = ? AND subscriber_person_url = ?", local, remote).First(&follower)
	return follower, result.Error
}

// GetFollowByID returns follow by ID
func (r *Repository) GetFollowByID(ctx context.Context, id string) (ApFollow, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollowByID")
	defer span.End()

	var follow ApFollow
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&follow)
	return follow, result.Error
}

// GetFollowerByID returns follower by ID
func (r *Repository) GetFollowerByID(ctx context.Context, id string) (ApFollower, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetFollowerByID")
	defer span.End()

	var follower ApFollower
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&follower)
	return follower, result.Error
}

// UpdateFollow updates follow
func (r *Repository) UpdateFollow(ctx context.Context, follow ApFollow) (ApFollow, error) {
	ctx, span := tracer.Start(ctx, "RepositoryUpdateFollow")
	defer span.End()

	result := r.db.WithContext(ctx).Save(&follow)
	return follow, result.Error
}

// GetAllFollows returns all Followers actions
func (r *Repository) GetAllFollowers(ctx context.Context) ([]ApFollower, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetAllFollows")
	defer span.End()

	var followers []ApFollower
	err := r.db.WithContext(ctx).Find(&followers).Error
	return followers, err
}

// Remove Follow action
func (r *Repository) RemoveFollow(ctx context.Context, followID string) (ApFollow, error) {
	ctx, span := tracer.Start(ctx, "RepositoryRemoveFollow")
	defer span.End()

	var follow ApFollow
	if err := r.db.WithContext(ctx).First(&follow, "id = ?", followID).Error; err != nil {
		return ApFollow{}, err
	}
	err := r.db.WithContext(ctx).Where("id = ?", followID).Delete(&ApFollow{}).Error
	if err != nil {
		return ApFollow{}, err
	}
	return follow, nil
}

// Remove Follower action
func (r *Repository) RemoveFollower(ctx context.Context, local, remote string) (ApFollower, error) {
	ctx, span := tracer.Start(ctx, "RepositoryRemoveFollower")
	defer span.End()

	var follower ApFollower
	err := r.db.WithContext(ctx).First(&follower, "publisher_user_id = ? AND subscriber_person_url = ?", local, remote).Error
	if err != nil {
		return ApFollower{}, err
	}

	err = r.db.WithContext(ctx).Where("publisher_user_id = ? AND subscriber_person_url = ?", local, remote).Delete(&ApFollower{}).Error
	if err != nil {
		return ApFollower{}, err
	}
	return follower, nil
}

// CreateApObjectReference creates reference
func (r *Repository) CreateApObjectReference(ctx context.Context, reference ApObjectReference) error {
	ctx, span := tracer.Start(ctx, "RepositoryCreateApObjectReference")
	defer span.End()

	return r.db.WithContext(ctx).Create(&reference).Error
}

// UpdateApObjectReference updates reference
func (r *Repository) UpdateApObjectReference(ctx context.Context, reference ApObjectReference) error {
	ctx, span := tracer.Start(ctx, "RepositoryUpdateApObjectReference")
	defer span.End()

	return r.db.WithContext(ctx).Save(&reference).Error
}

// GetApObjectReferenceByApObjectID returns reference by ap object ID
func (r *Repository) GetApObjectReferenceByApObjectID(ctx context.Context, apObjectID string) (ApObjectReference, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetApObjectReferenceByApObjectID")
	defer span.End()

	var references ApObjectReference
	err := r.db.WithContext(ctx).Where("ap_object_id = ?", apObjectID).First(&references).Error
	return references, err
}

// GetApObjectReferenceByCcObjectID returns reference by reference
func (r *Repository) GetApObjectReferenceByCcObjectID(ctx context.Context, ccObjectID string) (ApObjectReference, error) {
	ctx, span := tracer.Start(ctx, "RepositoryGetApObjectReferenceByCcObjectID")
	defer span.End()

	var references ApObjectReference
	err := r.db.WithContext(ctx).Where("cc_object_id = ?", ccObjectID).First(&references).Error
	return references, err
}

// DeleteApObjectReference deletes reference by ap object ID
func (r *Repository) DeleteApObjectReference(ctx context.Context, ApObjectID string) error {
	ctx, span := tracer.Start(ctx, "RepositoryDeleteApObjectReference")
	defer span.End()

	return r.db.WithContext(ctx).Where("ap_object_id = ?", ApObjectID).Delete(&ApObjectReference{}).Error
}

func (r *Repository) LoadKey(ctx context.Context, entity ApEntity) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(entity.Privatekey))
	if block == nil {
		return &rsa.PrivateKey{}, fmt.Errorf("failed to parse PEM block containing the key")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return &rsa.PrivateKey{}, fmt.Errorf("failed to parse DER encoded private key: " + err.Error())
	}

	return priv, nil
}

