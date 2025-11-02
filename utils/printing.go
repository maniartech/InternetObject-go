package utils

import (
	"encoding/json"
	"fmt"
	"log"
)

// PrettyPrint prints the formatted output for the specified struct
func PrettyPrint(val interface{}) {
	o, e := json.MarshalIndent(val, "", "  ")
	if e != nil {
		log.Panic(e.Error())
	}
	fmt.Print(string(o))
	fmt.Println()
}
