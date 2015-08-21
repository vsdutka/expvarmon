// expvar_test
package expvarmon

import (
	"testing"
)

func TestLoad(t *testing.T) {
	NewVar("var1", "Variable 1", "Bytes", "b")

	for j := 0; j < 1000; j++ {
		Add("var1", 1)
	}
	//	for i:=0; i<10; i++{
	//		go func(){

	//		}
	//	}

	res := Get("var1")
	if res != 1000 {
		t.Errorf("Get should be \"%s\", was \"%s\"", 1000, res)
	}
}
