package engine

import (
	"context"
	"encoding/json"
	"fmt"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	sjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
)

var Version = C.Version

type Instance struct {
	Box    *box.Box
	cancel context.CancelFunc
	clash  *clashapi.Server
}
type logSink struct{ fn func(string, string) }

func (s logSink) DisableColors() bool { return true }
func (s logSink) WriteMessage(l log.Level, m string) {
	if s.fn != nil {
		s.fn(log.FormatLevel(l), m)
	}
}
func Options(data []byte) (context.Context, option.Options, error) {
	ctx := service.ContextWithDefaultRegistry(include.Context(context.Background()))
	o, e := sjson.UnmarshalExtendedContext[option.Options](ctx, data)
	if o.Log != nil {
		o.Log.DisableColor = true
	}
	return ctx, o, e
}
func Start(data []byte, logger func(string, string)) (*Instance, error) {
	ctx, o, e := Options(data)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	b, e := box.New(box.Options{Context: ctx, Options: o, PlatformLogWriter: logSink{logger}})
	if e != nil {
		cancel()
		return nil, e
	}
	if e = b.Start(); e != nil {
		_ = b.Close()
		cancel()
		return nil, e
	}
	c, _ := service.FromContext[adapter.ClashServer](ctx).(*clashapi.Server)
	return &Instance{Box: b, cancel: cancel, clash: c}, nil
}
func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	i.cancel()
	return i.Box.Close()
}
func Validate(data []byte) error {
	ctx, o, e := Options(data)
	if e != nil {
		return e
	}
	b, e := box.New(box.Options{Context: ctx, Options: o, PlatformLogWriter: logSink{}})
	if e != nil {
		return e
	}
	_ = b.Close()
	return nil
}
func (i *Instance) Stats(detail bool) map[string]any {
	m := map[string]any{"upload": int64(0), "download": int64(0), "connections": 0}
	if i == nil || i.clash == nil {
		return m
	}
	t := i.clash.TrafficManager()
	up, down := t.Total()
	m["upload"] = up
	m["download"] = down
	m["connections"] = t.ConnectionsLen()
	if detail {
		b, e := json.Marshal(t.Connections())
		if e == nil {
			var a any
			_ = json.Unmarshal(b, &a)
			m["items"] = a
		}
	}
	return m
}
func (i *Instance) Dialer(tag string) (adapter.Outbound, error) {
	o, ok := i.Box.Outbound().Outbound(tag)
	if !ok {
		return nil, fmt.Errorf("找不到出站 %s", tag)
	}
	return o, nil
}
