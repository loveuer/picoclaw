package tools

import "testing"

func TestFacadeConstructorsRemainAvailable(t *testing.T) {
	if NewSerialTool() == nil {
		t.Fatal("NewSerialTool should return a tool")
	}
	if NewMessageTool() == nil {
		t.Fatal("NewMessageTool should return a tool")
	}
}
