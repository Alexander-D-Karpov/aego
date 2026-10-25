package backend

import "testing"

func TestDecodeStreamLines(t *testing.T) {
	text := Decode([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"t1","name":"mcp__aego__scene_tree","input":{"file":"a"}}]}}`))
	if len(text) != 2 || text[0].Text != "hi" || text[1].Tool != "scene_tree" || text[1].CallID != "t1" {
		t.Fatalf("assistant line %+v", text)
	}
	res := Decode([]byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok"}]}]}}`))
	if len(res) != 1 || res[0].Result != "ok" || res[0].CallID != "t1" {
		t.Fatalf("tool result %+v", res)
	}
	end := Decode([]byte(`{"type":"result","is_error":true,"result":"boom"}`))
	if len(end) != 2 || end[0].Kind != EvError || end[1].Kind != EvTurnEnd {
		t.Fatalf("result line %+v", end)
	}
	if Decode([]byte(`not json`)) != nil {
		t.Fatal("garbage gave events")
	}
}
