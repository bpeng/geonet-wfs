/*
cfgenv helps with setting configuration from the environment
as recommended by 12 factor applications.

Import for side effects will try to read the file env.list and set any
enviroment vars that are not already set.

    _ "github.com/GeoNet/cfg/cfgenv"

env.list should be key=value (the same format as used by Docker).

Configuration will happen before init().
*/
package cfgenv

import (
	"bufio"
	"io"
	"os"
	"strings"
)

/*
Will be non nil if there are errors reading env.list
*/
var Error = setEnv()

/*
Tries to read file and set environment vars that are not all ready set,
file should be in key=value format.
*/
func setEnv() (err error) {
	var f *os.File
	if f, err = os.Open("env.list"); err != nil {
		return
	}
	defer f.Close()

	var line string
	r := bufio.NewReader(f)

	for {
		if line, err = r.ReadString('\n'); err != nil {
			if err == io.EOF {
				err = nil
			}
			return
		}

		if !strings.HasPrefix(line, "\\#") && strings.Contains(line, "=") {
			line = strings.TrimSuffix(line, "\n")
			parts := strings.Split(line, "=")
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "" || value == "" {
				continue
			}

			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}

	}
}
