package di

import "net/http"

// App is the root of the dependency graph assembled by Wire.
type App struct {
	Server *http.Server
}
