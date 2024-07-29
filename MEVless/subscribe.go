package MEVless

import (
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/yu-org/yu/common"
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
	_, msg, err := c.ReadMessage()
	if err != nil {
		logrus.Error("SubscribeOrderCommitment ReadMessage failed: ", err)
		return
	}
	txnHash := common.BytesToHash(msg)
	txOrderByt, _, err := m.commitmentsDB.Get(txnHash.Bytes())
	if err != nil {
		logrus.Errorf("SubscribeOrderCommitment Get txOrder(%s) failed: %s", txnHash.String(), err)
		return
	}
	err = c.WriteMessage(websocket.TextMessage, txOrderByt)
	if err != nil {
		logrus.Error("SubscribeOrderCommitment WriteMessage failed: ", err)
		return
	}
}

func (m *MEVless) HandleSubscribe() {
	http.HandleFunc("/mev_less", m.SubscribeOrderCommitment)
	http.ListenAndServe(m.cfg.Addr, nil)
}
