package easytcp

import (
	"fmt"
	"github.com/DarthPestilane/easytcp/message"
	"github.com/DarthPestilane/easytcp/mock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer(&ServerOption{
		ReadTimeout:  0,
		WriteTimeout: 0,
		Codec:        &JsonCodec{},
	})
	assert.NotNil(t, s.accepting)
	assert.Equal(t, s.Packer, NewDefaultPacker())
	assert.Equal(t, s.Codec, &JsonCodec{})
}

func TestServer_Serve(t *testing.T) {
	goroutineNum := runtime.NumGoroutine()
	server := NewServer(&ServerOption{})
	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Equal(t, err, ErrServerStopped)
	}()
	<-server.accepting
	err := server.Stop()
	assert.NoError(t, err)
	// no goroutine leak
	for goroutineNum != runtime.NumGoroutine() {
		time.Sleep(time.Millisecond * 10)
	}
	// <-time.After(time.Millisecond * 10)
	// assert.Equal(t, goroutineNum, runtime.NumGoroutine()) // no goroutine leak
}

func TestServer_acceptLoop(t *testing.T) {
	t.Run("when everything's fine", func(t *testing.T) {
		server := NewServer(&ServerOption{
			SocketReadBufferSize:  1024,
			SocketWriteBufferSize: 1024,
		})
		address, err := net.ResolveTCPAddr("tcp", "localhost:0")
		assert.NoError(t, err)
		lis, err := net.ListenTCP("tcp", address)
		assert.NoError(t, err)
		server.Listener = lis
		go func() {
			err := server.acceptLoop()
			assert.Error(t, err)
		}()

		<-server.accepting

		// client
		cli, err := net.Dial("tcp", lis.Addr().String())
		assert.NoError(t, err)
		assert.NoError(t, cli.Close())
		assert.NoError(t, server.Stop())
	})
	t.Run("when accept returns a non-temporary error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := NewServer(&ServerOption{})

		listen := mock.NewMockListener(ctrl)
		listen.EXPECT().Accept().Return(nil, fmt.Errorf("some err"))
		server.Listener = listen
		done := make(chan struct{})
		go func() {
			assert.Error(t, server.acceptLoop())
			close(done)
		}()
		<-done
	})
	t.Run("when accept returns a temporary error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := NewServer(&ServerOption{})

		tempErr := mock.NewMockError(ctrl)
		tempErr.EXPECT().Error().MinTimes(1).Return("some err")
		i := 0
		tempErr.EXPECT().Temporary().MinTimes(1).DoAndReturn(func() bool {
			defer func() { i++ }()
			return i == 0 // returns true for the first time
		})

		listen := mock.NewMockListener(ctrl)
		listen.EXPECT().Accept().MinTimes(1).Return(nil, tempErr)
		server.Listener = listen
		go func() {
			assert.Error(t, server.acceptLoop())
		}()
		time.Sleep(time.Millisecond * 5)
		<-server.accepting
		time.Sleep(time.Millisecond * 20)
	})
}

func TestServer_Stop(t *testing.T) {
	server := NewServer(&ServerOption{})
	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Equal(t, err, ErrServerStopped)
	}()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", server.Listener.Addr().String())
	assert.NoError(t, err)

	<-time.After(time.Millisecond * 10)

	assert.NoError(t, server.Stop()) // stop server first
	assert.NoError(t, cli.Close())
}

func TestServer_handleConn(t *testing.T) {
	type TestReq struct {
		Param string
	}
	type TestResp struct {
		Success bool
	}

	// options
	codec := &JsonCodec{}
	packer := &DefaultPacker{}

	// server
	server := NewServer(&ServerOption{
		SocketReadBufferSize:  1,
		SocketWriteBufferSize: 1,
		SocketSendDelay:       true,
		Codec:                 codec,
		Packer:                packer,
		ReadBufferSize:        -1,
		WriteBufferSize:       -1,
	})

	// hooks
	server.OnSessionCreate = func(sess *Session) {
		fmt.Printf("session created | id: %s\n", sess.ID())
	}
	server.OnSessionClose = func(sess *Session) {
		fmt.Printf("session closed | id: %s\n", sess.ID())
	}

	// register route
	server.AddRoute(uint32(1), func(ctx *Context) (*message.Entry, error) {
		var reqData TestReq
		assert.NoError(t, ctx.Bind(&reqData))
		assert.EqualValues(t, 1, ctx.Message().ID)
		assert.Equal(t, reqData.Param, "hello test")
		return ctx.Response(uint32(2), &TestResp{Success: true})
	})
	// use middleware
	server.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx *Context) (*message.Entry, error) {
			defer func() {
				if r := recover(); r != nil {
					assert.Fail(t, "caught panic")
				}
			}()
			return next(ctx)
		}
	})

	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Equal(t, err, ErrServerStopped)
	}()
	defer func() { assert.NoError(t, server.Stop()) }()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", server.Listener.Addr().String())
	assert.NoError(t, err)
	defer func() { assert.NoError(t, cli.Close()) }()

	// client send msg
	reqData := &TestReq{Param: "hello test"}
	reqDataByte, err := codec.Encode(reqData)
	assert.NoError(t, err)
	msg := &message.Entry{
		ID:   uint32(1),
		Data: reqDataByte,
	}
	reqMsg, err := packer.Pack(msg)
	assert.NoError(t, err)
	_, err = cli.Write(reqMsg)
	assert.NoError(t, err)

	// client read msg
	respMsg, err := packer.Unpack(cli)
	assert.NoError(t, err)
	var respData TestResp
	assert.NoError(t, codec.Decode(respMsg.Data, &respData))
	assert.EqualValues(t, 2, respMsg.ID)
	assert.True(t, respData.Success)
}

func TestServer_NotFoundHandler(t *testing.T) {
	// server
	server := NewServer(&ServerOption{
		Packer: &DefaultPacker{},
	})
	server.NotFoundHandler(func(ctx *Context) (*message.Entry, error) {
		return ctx.Response(uint32(101), []byte("handler not found"))
	})
	go func() {
		err := server.Serve(":0")
		assert.Error(t, err)
		assert.Equal(t, err, ErrServerStopped)
	}()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", server.Listener.Addr().String())
	assert.NoError(t, err)
	defer func() { assert.NoError(t, cli.Close()) }()

	// send msg
	msg := &message.Entry{
		ID:   uint32(1),
		Data: []byte("test"),
	}
	reqMsg, err := server.Packer.Pack(msg)
	assert.NoError(t, err)
	_, err = cli.Write(reqMsg)
	assert.NoError(t, err)

	// read msg
	entry, err := server.Packer.Unpack(cli)
	assert.NoError(t, err)
	assert.EqualValues(t, entry.ID, 101)
	assert.Equal(t, entry.Data, []byte("handler not found"))
}
