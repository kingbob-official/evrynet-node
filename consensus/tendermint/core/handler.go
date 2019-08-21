package core

import (
	"log"
	"reflect"

	"github.com/ethereum/go-ethereum/consensus/tendermint"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.subscribeEvents()
	go c.handleEvents()

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.unsubscribeEvents()

	return nil
}

// ----------------------------------------------------------------------------

// Subscribe both internal and external events
func (c *core) subscribeEvents() {
	c.events = c.backend.EventMux().Subscribe(
		// external events
		tendermint.RequestEvent{},
		tendermint.MessageEvent{},
	)
	c.timeoutSub = c.backend.EventMux().Subscribe(
		timeoutEvent{},
	)
}

// Unsubscribe all events
func (c *core) unsubscribeEvents() {
	c.events.Unsubscribe()
	c.timeoutSub.Unsubscribe()
}

func (c *core) handleEvents() {
	// Clear state
	defer func() {
		c.handlerWg.Done()
	}()

	c.handlerWg.Add(1)

	for {
		select {
		case event, ok := <-c.events.Chan():
			if !ok {
				return
			}
			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case tendermint.RequestEvent:
				//TODO: Handle block proposal and remove this log
				log.Printf("--- Type of event.Data: %+v\n", reflect.TypeOf(ev))
				log.Printf("--- Value of event.Data: %+v\n", event.Data)
			case tendermint.MessageEvent:
				log.Printf("--- Type of event.Data: %+v\n", reflect.TypeOf(ev))
				log.Printf("--- Value of event.Data: %+v\n", ev.Payload)
				//TODO: Handle ev.Payload, if got error then call c.backend.Gossip()
			default:
				log.Printf("--- Unknow event :%v", ev)
			}
		case _, ok := <-c.timeoutSub.Chan():
			if !ok {
				return
			}
		}
	}
}
