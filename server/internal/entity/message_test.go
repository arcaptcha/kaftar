package entity

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMessageMarshal(t *testing.T) {
	var tmp any

	tmp = 5
	tmpByte, _ := json.Marshal(&tmp)
	fmt.Printf("%s\n", tmpByte)

	tmp = "hello"
	tmpByte, _ = json.Marshal(&tmp)
	fmt.Printf("%s\n", tmpByte)

	tmp = &SMSMessage{To: []string{"123"}, Text: ""}
	tmpByte, _ = json.Marshal(&tmp)
	fmt.Printf("%s\n", tmpByte)
	fmt.Printf("%v\n", tmpByte)
}

func TestConcreteMessage(t *testing.T) {
	m := &SMSMessage{
		To:   []string{"123"},
		Text: "hello",
	}
	msg, err := ConcreteMessage[*SMSMessage](m)
	fmt.Println(msg)
	fmt.Println(err)
}
