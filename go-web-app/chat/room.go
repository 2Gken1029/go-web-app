package main

import (
	"log"
	"main/trace"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/stretchr/objx"
)
type room struct {
	// 他のクライアントに転送するためのメッセージを保持するチャネル
	forward chan *message
	// チャットルームに参加しようとしているクライアントのためのチャネル
	join chan *client
	// チャットルームから退室しようとしているクライアントのためのチャネル
	leave chan *client
	// 在室しているすべてのクライアントが保持される
	clients map[*client] bool
	// チャットルーム上で行われた操作のログを受け取る
	tracer trace.Tracer
	// アバターの情報を取得する
	avatar Avatar
}

// newRoomはすぐに利用できるチャットルームを生成して返す
func newRoom() *room{
	return &room{
		forward: make(chan *message),
		join: make(chan *client),
		leave: make(chan *client),
		clients: make(map[*client] bool),
		tracer: trace.Off(),
	}
}


func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			// 参加
			r.clients[client] = true
			r.tracer.Trace("新しいクライアントが参加しました")
		case client := <-r.leave:
			// 退室
			delete(r.clients, client)
			close(client.send)
			r.tracer.Trace("クライアントが退室しました")
		case msg := <-r.forward:
			r.tracer.Trace("メッセージを受信しました: ", msg.Message)
			// すべてのクライアントにメッセージを転送
			for client := range r.clients {
				select {
				case client.send <- msg:
					// メッセージを送信
					r.tracer.Trace(" -- クライアントに送信されました")
				default:
					// 送信に失敗
					delete(r.clients, client)
					close(client.send)
					r.tracer.Trace(" -- 送信に失敗しました。クライアントをクリーンアップします")
				}
			}
		}
	}
}

const (
	socketBufferSize = 1024
	messageBufferSize = 256
)
var upgrader = &websocket.Upgrader{
	ReadBufferSize: socketBufferSize, WriteBufferSize: socketBufferSize,
}
func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request){
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Fatal("ServerHTTP:",err)
		return
	}

	authCookie, err := req.Cookie("auth")
	if err != nil {
		log.Fatal("クッキーの取得に失敗しました", err)
		return
	}
	client := &client{
		socket: socket,
		send: make(chan *message, messageBufferSize),
		room: r,
		userData: objx.MustFromBase64(authCookie.Value),
	}
	r.join <- client
	defer func() {r.leave <- client} ()
	go client.write()
	client.read()
}