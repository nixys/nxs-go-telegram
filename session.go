package tg

import (
	"fmt"
)

type SessionState struct {
	state string
}

// session it is a session context structure
type session struct {
	chatID int64
	userID int64
	host   string
}

var (

	// sessionDestroy it's a 'destroy' session state
	sessionDestroy SessionState = SessionState{"internal:destroy"}

	// sessionBreak it's a 'break' session state
	sessionBreak SessionState = SessionState{""}
)

// data contains session data
type data struct {
	State string                 `json:"state"`
	Slots map[string]interface{} `json:"slots"`
}

func SessStateBreak() SessionState {
	return sessionBreak
}

func SessStateDestroy() SessionState {
	return sessionDestroy
}

func SessState(stateName string) SessionState {
	return SessionState{"user:" + stateName}
}

// sessionInit initiates session
func sessionInit(chatID, userID int64, host string) session {

	var s session

	s.chatID = chatID
	s.userID = userID
	s.host = host

	return s
}

// chatIDGet gets current session chat ID
func (s *session) chatIDGet() int64 {
	return s.chatID
}

// userIDGet gets current session user ID
func (s *session) userIDGet() int64 {
	return s.userID
}

// destroy destroys current session
func (s *session) destroy() error {

	r, err := redisConnect(s.host)
	if err != nil {
		return err
	}
	defer r.close()

	return r.sessDel(s.chatID, s.userID)
}

// stateGet gets current session state
func (s *session) stateGet() (SessionState, bool, error) {

	r, err := redisConnect(s.host)
	if err != nil {
		return sessionBreak, false, err
	}
	defer r.close()

	d, e, err := r.sessGet(s.chatID, s.userID)
	if err != nil {
		return sessionBreak, false, err
	}

	return SessionState{d.State}, e, nil
}

// stateSet sets session into state `state`.
// Starts new session if not exist
func (s *session) stateSet(state SessionState) error {

	r, err := redisConnect(s.host)
	if err != nil {
		return err
	}
	defer r.close()

	d, e, err := r.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		d = data{
			State: state.state,
			Slots: make(map[string]interface{}),
		}
	} else {
		d.State = state.state
	}

	return r.sessSave(s.chatID, s.userID, d)
}

// slotSave saves data into specified slot
func (s *session) slotSave(slot string, data interface{}) error {

	r, err := redisConnect(s.host)
	if err != nil {
		return err
	}
	defer r.close()

	d, e, err := r.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		return fmt.Errorf("session does not exist")
	}

	d.Slots[slot] = data

	return r.sessSave(s.chatID, s.userID, d)
}

// slotGet gets data from specified slot
func (s *session) slotGet(slot string) (interface{}, bool, error) {

	r, err := redisConnect(s.host)
	if err != nil {
		return nil, false, err
	}
	defer r.close()

	d, e, err := r.sessGet(s.chatID, s.userID)
	if err != nil {
		return nil, false, err
	}

	if e == false {
		return nil, false, fmt.Errorf("session does not exist")
	}

	data, e := d.Slots[slot]

	return data, e, nil
}

// slotDel geletes spcified slot
func (s *session) slotDel(slot string) error {

	r, err := redisConnect(s.host)
	if err != nil {
		return err
	}
	defer r.close()

	d, e, err := r.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		return fmt.Errorf("session does not exist")
	}

	delete(d.Slots, slot)

	return r.sessSave(s.chatID, s.userID, d)
}
