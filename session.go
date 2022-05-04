package tg

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

type SessionState struct {
	state string
}

// session it is a session context structure
type Session struct {
	chatID      int64
	userID      int64
	userName    string
	updateChain *UpdateChain
	redis       *redis
}

var (

	// sessionDestroy it's a 'destroy' session state
	sessionDestroy SessionState = SessionState{"internal:destroy"}

	// sessionBreak it's a 'break' session state
	sessionBreak SessionState = SessionState{""}
)

// data contains session data
type data struct {
	State string            `json:"state"`
	Slots map[string][]byte `json:"slots"`
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

func (s SessionState) String() string {
	return s.state
}

// sessionInit initiates session
func sessionInit(uc UpdateChain, redisHost string) (*Session, error) {

	var err error

	// Skip processing zero-len update chain
	if len(uc.updates) == 0 {
		return nil, ErrUpdateChainZeroLen
	}

	s := new(Session)

	s.updateChain = &uc

	// Get chat and user IDs from first update from chain
	s.chatID, s.userID = updateIDsGet(s.updateChain.updates[0])

	// Get user name from first update from chain
	s.userName = updateUserNameGet(s.updateChain.updates[0])

	s.redis, err = redisConnect(redisHost)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// close closes Redis connection for session
func (s *Session) close() error {
	return s.redis.close()
}

// ChatIDGet gets current session chat ID
func (s *Session) ChatIDGet() int64 {
	return s.chatID
}

// UserIDGet gets current session user ID
func (s *Session) UserIDGet() int64 {
	return s.userID
}

// UserNameGet gets current session user name
func (s *Session) UserNameGet() string {
	return s.userName
}

// UpdateChain gets update chain from session
func (s *Session) UpdateChain() *UpdateChain {
	return s.updateChain
}

// SlotSave saves data into specified slot
func (s *Session) SlotSave(slot string, data interface{}) error {

	var buf bytes.Buffer

	d, e, err := s.redis.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		return fmt.Errorf("session does not exist")
	}

	// Encode data to bytes
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return err
	}

	d.Slots[slot] = buf.Bytes()

	return s.redis.sessSave(s.chatID, s.userID, d)
}

// SlotGet gets data from specified slot
func (s *Session) SlotGet(slot string, data interface{}) (bool, error) {

	d, e, err := s.redis.sessGet(s.chatID, s.userID)
	if err != nil {
		return false, err
	}

	if e == false {
		return false, fmt.Errorf("session does not exist")
	}

	ds, e := d.Slots[slot]
	if e == false {
		return false, nil
	}

	if err := gob.NewDecoder(bytes.NewBuffer(ds)).Decode(data); err != nil {
		return false, err
	}

	return true, nil
}

// SlotDel deletes spcified slot
func (s *Session) SlotDel(slot string) error {

	d, e, err := s.redis.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		return fmt.Errorf("session does not exist")
	}

	delete(d.Slots, slot)

	return s.redis.sessSave(s.chatID, s.userID, d)
}

// stateProcessing processes current session state.
// It's initial point to route processing into appropriate state
// in accordance with update chain
func (s *Session) stateProcessing(t *Telegram) error {

	// Check `update` is a defined command
	b, err := s.stateCommandProcessing(t)
	if b == true {
		// If command were found
		return err
	}

	switch s.UpdateChain().TypeGet() {
	case UpdateTypeMessage:
		return s.stateMessageProcessing(t)
	case UpdateTypeCallback:
		return s.stateCallbackProcessing(t)
	}

	return nil
}

// stateInitProcessing processes session init state
func (s *Session) stateInitProcessing(t *Telegram) error {

	var ns SessionState

	if t.description.InitHandler == nil {
		return nil
	}

	// Call initHandler
	r, err := t.description.InitHandler(t, s)
	if err != nil {

		if t.description.ErrorHandler == nil {
			return err
		}

		r, err := t.description.ErrorHandler(t, s, err)
		if err != nil {
			return err
		}

		ns = r.NextState
	} else {
		ns = r.NextState
	}

	return s.stateSwitch(t, ns, 0)
}

// stateCommandProcessing lookups and processes command (if described) by message text from Telegram update.
func (s *Session) stateCommandProcessing(t *Telegram) (bool, error) {

	var ns SessionState

	// Check update contains command
	cmd, args := s.UpdateChain().commandCheck()
	if len(cmd) == 0 {
		return false, nil
	}

	// Check specified command defined in bot description
	c := t.description.commandLookup(cmd)
	if c == nil {
		return false, nil
	}

	// Check handler defined for command
	if c.Handler == nil {
		return true, nil
	}

	r, err := c.Handler(t, s, cmd, args)
	if err != nil {

		if t.description.ErrorHandler == nil {
			return true, err
		}

		r, err := t.description.ErrorHandler(t, s, err)
		if err != nil {
			return true, err
		}

		ns = r.NextState
	} else {
		ns = r.NextState
	}

	return true, s.stateSwitch(t, ns, 0)
}

// stateMessageProcessing processes update chain with `message` type
func (s *Session) stateMessageProcessing(t *Telegram) error {

	var ns SessionState

	// Get current session
	cs, e, err := s.StateGet()
	if err != nil {
		return err
	}

	// If session does not exist
	if e == false {
		return s.stateInitProcessing(t)
	}

	// Get state description
	state, b := t.description.States[cs]
	if b == false {
		return ErrDescriptionStateMissing
	}

	if state.MessageHandler == nil {
		return nil
	}

	r, err := state.MessageHandler(t, s)
	if err != nil {

		if t.description.ErrorHandler == nil {
			return err
		}

		r, err := t.description.ErrorHandler(t, s, err)
		if err != nil {
			return err
		}

		ns = r.NextState
	} else {
		ns = r.NextState
	}

	return s.stateSwitch(t, ns, 0)
}

// stateCallbackProcessing processes update chain with `callback` type
func (s *Session) stateCallbackProcessing(t *Telegram) error {

	var ns SessionState

	cbs, identifier, err := s.UpdateChain().callbackSessionStateGet()
	if err != nil {
		return err
	}

	// Check the button contains special states
	switch cbs {
	case
		sessionBreak,
		sessionDestroy:
		return s.stateSwitch(t, cbs, s.UpdateChain().MessagesIDGet())
	}

	// Get state description
	state, b := t.description.States[cbs]
	if b == false {
		return ErrDescriptionStateMissing
	}

	if state.CallbackHandler == nil {
		return nil
	}

	r, err := state.CallbackHandler(t, s, identifier)
	if err != nil {

		if t.description.ErrorHandler == nil {
			return err
		}

		r, err := t.description.ErrorHandler(t, s, err)
		if err != nil {
			return err
		}

		ns = r.NextState
	} else {
		ns = r.NextState
	}

	return s.stateSwitch(t, ns, s.UpdateChain().MessagesIDGet())
}

func (s *Session) stateSwitch(t *Telegram, newState SessionState, messageID int) error {

	var mID int

	switch newState {
	case sessionBreak:
		return nil
	case sessionDestroy:
		return s.destroy()
	}

	state, b := t.description.States[newState]
	if b == false {
		return ErrDescriptionStateMissing
	}

	// Put session into new state
	if err := s.stateSet(newState); err != nil {
		return err
	}

	if state.StateHandler == nil {
		// Do nothing if state handler not defined
		return nil
	}

	hr, err := state.StateHandler(t, s)
	if err != nil {

		if t.description.ErrorHandler == nil {
			return err
		}

		r, err := t.description.ErrorHandler(t, s, err)
		if err != nil {
			return err
		}

		return s.stateSwitch(t, r.NextState, 0)
	}

	if hr.StickMessage == true {
		mID = messageID
	}

	// Send message to user if set
	if len(hr.Message) > 0 {

		msgs, err := t.SendMessage(s.ChatIDGet(), mID, SendMessageData{
			Message:     hr.Message,
			Buttons:     hr.Buttons,
			ButtonState: newState,
		})
		if err != nil {
			return err
		}

		if state.SentHandler != nil {
			if err := state.SentHandler(t, s, msgs); err != nil {
				return err
			}
		}
	}

	// Do not switch next state if message handler defined
	if state.MessageHandler != nil {
		return nil
	}

	return s.stateSwitch(t, hr.NextState, mID)
}

// destroy destroys current session
func (s *Session) destroy() error {
	return s.redis.sessDel(s.chatID, s.userID)
}

// stateGet gets current session state
func (s *Session) StateGet() (SessionState, bool, error) {

	d, e, err := s.redis.sessGet(s.chatID, s.userID)
	if err != nil {
		return sessionBreak, false, err
	}

	return SessionState{d.State}, e, nil
}

// stateSet sets session into state `state`.
// Starts new session if not exist
func (s *Session) stateSet(state SessionState) error {

	d, e, err := s.redis.sessGet(s.chatID, s.userID)
	if err != nil {
		return err
	}

	if e == false {
		d = data{
			State: state.state,
			Slots: make(map[string][]byte),
		}
	} else {
		d.State = state.state
	}

	return s.redis.sessSave(s.chatID, s.userID, d)
}
