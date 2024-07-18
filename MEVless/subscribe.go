package MEVless

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"net/http"
)

var upgrader = websocket.Upgrader{}

func (m *MEVless) SubscribeOrderCommitment(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Errorf("SubscribeOrderCommitment: websocket upgrade failed: %s", err)
		return
	}
	defer c.Close()
	for {
		select {
		case oc := <-m.orderCommitments:
			byt, err := json.Marshal(oc)
			if err != nil {
				logrus.Error("SubscribeOrderCommitment json.Marshal: ", err)
				break
			}
			err = c.WriteMessage(websocket.TextMessage, byt)
			if err != nil {
				logrus.Error("SubscribeOrderCommitment write:", err)
				break
			}
		}

	}
}

func (m *MEVless) HandleSubscribe() {
	http.HandleFunc("/mev_less", m.SubscribeOrderCommitment)
	http.ListenAndServe(m.cfg.Addr, nil)
}
