package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmcgn/lumina/internal/widget"
)

func (h *Hub) handleAction(c *Client, msg Envelope) {
	reply := func(res ActionResult) {
		c.Send(encode(MsgActionResult, msg.Widget, msg.ID, res))
	}

	var p ActionPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil || p.Name == "" {
		reply(ActionResult{Error: "bad action payload"})
		return
	}
	r := h.runner(msg.Widget)
	if r == nil {
		reply(ActionResult{Error: "unknown widget"})
		return
	}
	if r.conf.ReadOnly {
		reply(ActionResult{Error: "widget is read-only"})
		return
	}
	actor, ok := r.prov.(widget.Actor)
	if !ok || r.startErr != nil {
		reply(ActionResult{Error: "widget does not support actions"})
		return
	}

	ctx, cancel := context.WithTimeout(h.ctx, actionTimeout)
	defer cancel()
	data, err := safeAction(ctx, actor, p.Name, p.Params)
	if err != nil {
		r.log.Warn("action failed", "action", p.Name, "err", err)
		reply(ActionResult{Error: err.Error()})
		return
	}
	reply(ActionResult{OK: true, Data: data})
	r.requestRefresh()
}

func safeAction(ctx context.Context, a widget.Actor, name string, params json.RawMessage) (data any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in provider: %v", rec)
		}
	}()
	return a.Action(ctx, name, params)
}
