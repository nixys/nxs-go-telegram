package tg

import (
	"time"
)

// queue it is a queue context structure
type queue struct {
	redis        *redis
	waitInterval time.Duration
}

type queueChain struct {
}

// queueInit initiates queue
func queueInit(host string, waitInterval time.Duration) (queue, error) {

	var (
		q   queue
		err error
	)

	q.redis, err = redisConnect(host)
	if err != nil {
		return q, err
	}

	q.waitInterval = waitInterval

	return q, nil
}

func (q *queue) close() error {
	return q.redis.close()
}

// add adds element into queue
func (q *queue) add(chatID, userID int64, update Update) error {

	if err := q.redis.queueMetaAdd(chatID, userID, time.Now().Add(q.waitInterval)); err != nil {
		return err
	}

	if err := q.redis.queueUpdateAdd(chatID, userID, update); err != nil {
		return err
	}

	return nil
}

// chainGet finds available queue and get update chain
func (q *queue) chainGet() (UpdateChain, error) {

	var uc UpdateChain

	qm, err := q.redis.queueMetasGet()
	if err != nil {
		return UpdateChain{}, err
	}

	for _, m := range qm {
		if time.Now().After(m.waitTill) == true {

			// Delete meta for this queue to prevent queue race with other goroutines
			i, err := q.redis.queueMetaDel(m.chatID, m.userID)
			if err != nil {
				return uc, err
			}

			if i == 0 {
				// If other goroutine lock the queue first
				continue
			}

			u, err := q.redis.queueUpdatesGet(m.chatID, m.userID)
			if err != nil {
				return uc, err
			}

			uc.add(u)

			return uc, nil
		}
	}

	return UpdateChain{}, nil
}
