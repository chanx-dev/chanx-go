package protocol_test

import (
	"fmt"

	"github.com/huynguyengl99/chanx-go/protocol"
)

// Servers in every language build the same group name from the same parts, so they
// meet in one group over a shared layer.
func ExampleSafeGroupName() {
	name, _ := protocol.SafeGroupName("RoomTopic", "room:lobby")
	fmt.Println(name)
	// Output: RoomTopic.room-lobby
}

func ExampleParsePattern() {
	p := protocol.MustParsePattern("room:{room_name}")
	params, ok := p.Match("room:lobby")
	topic, _ := p.Format(map[string]string{"room_name": "kitchen"})
	fmt.Println(params, ok, topic)
	// Output: map[room_name:lobby] true room:kitchen
}

// Camelize and Decamelize case keys as pyhumps does, for Config.Camelize.
func ExampleCamelize() {
	fmt.Println(protocol.Camelize("room_name"), protocol.Decamelize("APIResponse"))
	// Output: roomName api_response
}
