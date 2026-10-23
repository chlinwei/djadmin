package rabbitmq

import "testing"

func TestTopologyOnlyDeclaresLogCollection(t *testing.T) {
	if len(Routes) != 1 || Routes[0] != LogCollectRoute {
		t.Fatalf("unexpected active queue routes: %+v", Routes)
	}
}
