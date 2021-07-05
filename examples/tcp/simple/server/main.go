package main

import (
	"fmt"
	"github.com/DarthPestilane/easytcp"
	"github.com/DarthPestilane/easytcp/examples/fixture"
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/DarthPestilane/easytcp/router"
	"github.com/DarthPestilane/easytcp/server"
	"github.com/DarthPestilane/easytcp/session"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
}

func main() {
	// go printGoroutineNum()

	s := easytcp.NewTCPServer(&server.TCPOption{
		SocketRWBufferSize: 1024 * 1024,
		ReadTimeout:        time.Second * 10,
		WriteTimeout:       time.Second * 10,
		MsgPacker:          &packet.DefaultPacker{}, // with default packer
		MsgCodec:           nil,                     // without codec
		ReadBufferSize:     0,
		WriteBufferSize:    0,
	})
	s.OnSessionCreate = func(sess session.Session) {
		log.Infof("session created: %s", sess.ID())
	}
	s.OnSessionClose = func(sess session.Session) {
		log.Warnf("session closed: %s", sess.ID())
	}

	// register global middlewares
	s.Use(fixture.RecoverMiddleware(log), logMiddleware)

	// register a route
	s.AddRoute(fixture.MsgIdPingReq, func(ctx *router.Context) (*packet.MessageEntry, error) {
		return ctx.Response(fixture.MsgIdPingAck, "pong, pong, pong")
	})

	go func() {
		if err := s.Serve(fixture.ServerAddr); err != nil {
			log.Errorf("serve err: %s", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	<-sigCh
	if err := s.Stop(); err != nil {
		log.Errorf("server stopped err: %s", err)
	}
}

func logMiddleware(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) (resp *packet.MessageEntry, err error) {
		log.Infof("rec <<< | id:(%d) size:(%d) data: %s", ctx.MsgID(), ctx.MsgSize(), ctx.MsgData())
		defer func() {
			if err != nil || resp == nil {
				return
			}
			log.Infof("snd >>> | id:(%d) size:(%d) data: %s", resp.ID, len(resp.Data), resp.Data)
		}()
		return next(ctx)
	}
}

// nolint: deadcode, unused
func printGoroutineNum() {
	for {
		fmt.Println("goroutine num: ", runtime.NumGoroutine())
		time.Sleep(time.Second)
	}
}
