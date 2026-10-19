package facilities

import "testing"

func TestPhysicalRoomAliases(t *testing.T) {
	a := Key(26, 56, "laboratory", "IT Lab 1")
	b := Key(26, 73, "laboratory", "IT Laboratory 1")
	if a != b {
		t.Fatalf("known CITE aliases should collide: %s != %s", a, b)
	}
	if Key(26, 56, "laboratory", "IT Lab 1") == Key(27, 77, "classroom", "IT Lab 1") {
		t.Fatal("different buildings/kinds cannot collide")
	}
	if Key(26, 11, "laboratory", "") == Key(26, 12, "laboratory", "") {
		t.Fatal("unnamed rooms must remain distinct")
	}
}
