//go:build android && cgo

package main

import "os"

// invokeAction already runs handleAction in its own goroutine, so file ops
// do not need an inner one.
func handleDelFile(path string, result ActionResult) {
	safePath, err := resolveAllowedPath(path)
	if err != nil {
		result.error(err.Error())
		return
	}
	fileInfo, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			result.success("")
			return
		}
		result.error(err.Error())
		return
	}
	if fileInfo.IsDir() {
		err = os.RemoveAll(safePath)
	} else {
		err = os.Remove(safePath)
	}
	if err != nil {
		result.error(err.Error())
		return
	}
	result.success("")
}
