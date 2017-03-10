GOOS=linux GOARCH=386 go build -o xengine_hub_linux32.exe xengine_hub.go
GOOS=linux GOARCH=amd64 go build -o xengine_hub_linux64.exe xengine_hub.go
GOOS=windows GOARCH=386 go build -o xengine_hub_windows32.exe xengine_hub.go
GOOS=windows GOARCH=amd64 go build -o xengine_hub_windows64.exe xengine_hub.go
