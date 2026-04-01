package store

import (
	"encoding/binary"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

var bucketName = []byte("seen")

// Store tracks which HN item IDs have already been sent
type Store interface {
	HasSeen(id int) bool
	MarkSeen(id int) error
	Prune(olderThan time.Duration) error
	Close() error
}

// BoltStore implements Store using BoltDB
type BoltStore struct {
	db *bbolt.DB
}

// NewBoltStore opens (or creates) a BoltDB at the given path
func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt db at %s: %w", path, err)
	}

	// Ensure bucket exists
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating bucket: %w", err)
	}

	return &BoltStore{db: db}, nil
}

// HasSeen returns true if the item ID has been recorded
func (s *BoltStore) HasSeen(id int) bool {
	found := false
	_ = s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return nil
		}
		v := b.Get(idToBytes(id))
		found = v != nil
		return nil
	})
	return found
}

// MarkSeen records the item ID with the current timestamp
func (s *BoltStore) MarkSeen(id int) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucketName)
		}
		return b.Put(idToBytes(id), timeToBytes(time.Now()))
	})
}

// Prune removes entries older than the given duration
func (s *BoltStore) Prune(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return nil
		}

		var toDelete [][]byte
		err := b.ForEach(func(k, v []byte) error {
			t := bytesToTime(v)
			if t.Before(cutoff) {
				toDelete = append(toDelete, k)
			}
			return nil
		})
		if err != nil {
			return err
		}

		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// Close closes the underlying BoltDB
func (s *BoltStore) Close() error {
	return s.db.Close()
}

func idToBytes(id int) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(id))
	return buf
}

func timeToBytes(t time.Time) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(t.Unix()))
	return buf
}

func bytesToTime(b []byte) time.Time {
	if len(b) < 8 {
		return time.Time{}
	}
	unix := int64(binary.BigEndian.Uint64(b))
	return time.Unix(unix, 0)
}
