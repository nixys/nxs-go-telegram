package tg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	rds "github.com/go-redis/redis"
)

type redis struct {
	client *rds.Client
}

type queueMeta struct {
	chatID   int64
	userID   int64
	waitTill time.Time
}

const (
	sessionKey      = "sess"
	queueMetaKey    = "meta"
	queueUpdatesKey = "updates"
)

// connect connects to Redis
func redisConnect(host string) (*redis, error) {

	r := new(redis)

	client := rds.NewClient(&rds.Options{
		Addr:         host,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		PoolTimeout:  30 * time.Second,
	})

	p := client.Ping()

	if p.Err() != nil {
		return r, p.Err()
	}

	r.client = client

	return r, nil
}

// close closes Redis connection
func (r *redis) close() error {
	return r.client.Close()
}

// sessSave saves the session into Redis
func (r *redis) sessSave(chatID, userID int64, d data) error {

	b, err := json.Marshal(d)
	if err != nil {
		return err
	}

	s := r.client.HSet(sessionKey, strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10), b)
	if s.Err() != nil {
		return s.Err()
	}

	return nil
}

// sessGet gets session from Redis
func (r *redis) sessGet(chatID, userID int64) (data, bool, error) {

	var d data

	s := r.client.HGet(sessionKey, strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10))
	if s.Err() != nil {
		if s.Err() == rds.Nil {
			// Key not found
			return d, false, nil
		}
		return d, false, s.Err()
	}

	b, err := s.Bytes()
	if err != nil {
		return d, false, err
	}

	if err := json.Unmarshal(b, &d); err != nil {
		return d, false, err
	}

	return d, true, nil
}

// sessDel deletes session from Redis
func (r *redis) sessDel(chatID, userID int64) error {

	// Delete session
	s := r.client.HDel(sessionKey, strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10))
	if s.Err() != nil {
		if s.Err() == rds.Nil {
			// Key not found
			return nil
		}
		return s.Err()
	}

	// Delete meta
	if _, err := r.queueMetaDel(chatID, userID); err != nil {
		return err
	}

	if err := r.queueUpdateDel(chatID, userID); err != nil {
		return err
	}

	return nil
}

// queueMetaAdd adds or updates specified meta
func (r *redis) queueMetaAdd(chatID, userID int64, waitTill time.Time) error {

	t, _ := waitTill.MarshalJSON()

	s := r.client.HSet(queueMetaKey, strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10), t)
	if s.Err() != nil {
		return s.Err()
	}

	return nil
}

// queueMetasGet gets all meta from Redis
func (r *redis) queueMetasGet() ([]queueMeta, error) {

	var qm []queueMeta

	metas := r.client.HGetAll(queueMetaKey)
	if metas.Err() != nil {
		return qm, metas.Err()
	}

	for k, v := range metas.Val() {

		var t time.Time

		err := t.UnmarshalJSON([]byte(v))
		if err != nil {
			return qm, err
		}

		ids := strings.Split(k, ":")
		if len(ids) != 2 {
			return qm, fmt.Errorf("wrong queue meta field")
		}

		chatID, err := strconv.ParseInt(ids[0], 10, 64)
		if err != nil {
			return qm, err
		}

		userID, err := strconv.ParseInt(ids[1], 10, 64)
		if err != nil {
			return qm, err
		}

		qm = append(qm, queueMeta{
			chatID:   chatID,
			userID:   userID,
			waitTill: t,
		})
	}

	return qm, nil
}

// queueMetaDel deletes specified meta
func (r *redis) queueMetaDel(chatID, userID int64) (int64, error) {

	s := r.client.HDel(queueMetaKey, strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10))
	if s.Err() != nil {
		return 0, s.Err()
	}

	return s.Val(), nil
}

// queueUpdateAdd adds new update into specified list
func (r *redis) queueUpdateAdd(chatID, userID int64, update Update) error {

	b, err := json.Marshal(update)
	if err != nil {
		return err
	}

	s := r.client.RPush(queueUpdatesKey+":"+strconv.FormatInt(chatID, 10)+":"+strconv.FormatInt(userID, 10), b)
	if s.Err() != nil {
		return s.Err()
	}

	return nil
}

// queueUpdatesGet gets all updates from specified list
func (r *redis) queueUpdatesGet(chatID, userID int64) ([]Update, error) {

	var updates []Update

	l := r.client.LLen(queueUpdatesKey + ":" + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10))
	if l.Err() != nil {
		return updates, l.Err()
	}

	for len := l.Val(); len > 0; len-- {

		var update Update

		s := r.client.LPop(queueUpdatesKey + ":" + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10))
		if s.Err() != nil {
			return updates, s.Err()
		}

		if err := json.Unmarshal([]byte(s.Val()), &update); err != nil {
			return updates, err
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// queueUpdateDel deletes specified list
func (r *redis) queueUpdateDel(chatID, userID int64) error {

	// Delete queue
	s := r.client.Del(queueUpdatesKey + ":" + strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(userID, 10))
	if s.Err() != nil {
		if s.Err() == rds.Nil {
			// Key not found
			return nil
		}
		return s.Err()
	}

	return nil
}
