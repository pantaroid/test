package main

import (
  "github.com/braintree/manners"
  "gopkg.in/gin-gonic/gin.v1"
  "net/http"
  "net"
  "fmt"
  "os"
  "os/signal"
  "io"
  "io/ioutil"
  "syscall"
  "strings"
  "strconv"
)

type UploadedFile struct {
  Name string
  Time string//time.Time
}

type ServiceServer struct {
  Port int
  Status int
  Name string
  Addr string
  Node Node
}

type Node struct {
  Status int
  Name string
  Addr string
  ServiceServers []ServiceServer
}

type HubInfo struct {
  Nodes []Node
  Domains map[string][]ServiceServer
}

const (
  dateTimeLayout = "2006-01-02 15:04:05"
)

func index(c *gin.Context, h *HubInfo) {
  files, _ := ioutil.ReadDir("./files")
  size := len(files)
  lists := make([]UploadedFile, size)
  for i := 0; i < size; i++ {
    lists[i] = UploadedFile{ files[i].Name(), files[i].ModTime().Format(dateTimeLayout) }
  }
  c.HTML(http.StatusOK, "index.tmpl", gin.H {
    "files": lists,
    "nodes": h.Nodes,
  })
}

func execute(c *gin.Context, h *HubInfo) {
  var json map[string]interface{}
  err := c.BindJSON(&json)
  if err != nil {
    fmt.Printf("Error: %s\n", err)
    c.Status(http.StatusNotFound)
    return
  }
  fmt.Printf("%#v\n", json)

  switch json["key"] {
    case "deleteFile":
      os.Remove("./files/" + json["name"].(string))
    default:
  }

  index(c, h)
  //c.Redirect(http.StatusMovedPermanently, "/")
}

func upload(c *gin.Context) {
  file, header, err := c.Request.FormFile("file")
  if err == nil {
    // get filename
    filename := header.Filename
    // create file
    out, err := os.Create("./files/" + filename)
    if err == nil {
      defer out.Close()
      // copy from temporary
      _, err = io.Copy(out, file)
    }
  }

  if err != nil {
    fmt.Printf("Error: %s\n", err)
  }
  c.Redirect(http.StatusMovedPermanently, "/")
}

func download(c *gin.Context) {
  fileName := c.Param("file") // c.PostForm("name")
  file, err := os.Open("./files/" + fileName)
  if err != nil {
    fmt.Printf("Error: %s\n", err)
    c.Status(http.StatusNotFound)
    return
  }
  defer file.Close()

  bytes, err := ioutil.ReadAll(file)
  if err != nil {
    fmt.Printf("Error: %s\n", err)
    c.Status(http.StatusNotFound)
    return
  }

  c.Header("Content-Disposition", "attachment; filename=" + fileName )
  c.Data(http.StatusOK, "application/zip", bytes)
}

func main() {

  cInfo := make(chan *HubInfo)
  fInfo := func(fn func(*HubInfo)) {
    info :=  <- cInfo
    fn(info)
    cInfo <- info
  }


  {// ResourceMaster
    go func() {
      var info *HubInfo
      info = &HubInfo{}
      // debug
      info.Nodes = append(info.Nodes, Node { Status: 0, Name: "TEST", Addr: "Sample" })

      for {
        select {
          case cInfo <- info:
            info = <- cInfo
        }
      }
    }()
  }

  {// CommunicationServer
    fmt.Println("UDP START!!")
    addr, err := net.ResolveUDPAddr("udp", ":51702")
    if err != nil {
      fmt.Printf("Error: %s\n", err)
      return
    }
    conn, err := net.ListenUDP("udp", addr)
    if err != nil {
      fmt.Printf("Error: %s\n", err)
      return
    }
    defer conn.Close()

    go func() {
      buf := make([]byte, 1024)
      for {
        rlen, remote, err := conn.ReadFromUDP(buf)
        if err == nil {
          message := string(buf[:rlen])
          //rlen, err = conn.WriteToUDP([]byte(s), remote)
          fmt.Printf("Receive [%v]: %v\n", remote, message)

          // test
          if strings.HasPrefix(message, "A") {
            parts := strings.Split(message, ":")
            for index, element := range parts {
              fmt.Printf("  %d: [%s]\n", index, element)
            }

            fInfo(func(info *HubInfo) {
              status, _ := strconv.Atoi(parts[1])
              info.Nodes = append(info.Nodes, Node {
                Status: status,
                Name: parts[2],
                Addr: parts[3],
              })
            })
          }
        }

        //n, addr, err := ServerConn.ReadFromUDP(buf)
        //fmt.Println("Received ",string(buf[0:n]), " from ",addr)
        //if err != nil {
        //  fmt.Println("Error: ",err)
        //} 
      }
    }()
  }

  {// WebServer
    // initailize
    router := gin.Default()
    // load templates
    router.LoadHTMLGlob("./templates/*")
    // resources
    router.StaticFile("/script.js", "./resources/script.js")
    // root
    //router.GET("/", index)
    router.GET("/", func(c *gin.Context) {
      fInfo(func(info *HubInfo) {
        fmt.Printf("HubInfo.Nodes [%v]\n", info.Nodes)
        index(c, info)
      })
   })

    // loaders
    router.POST("/upload", upload)
    router.GET("/download/:file", download)
    // execute
    router.POST("/execute", func(c *gin.Context) {
      fInfo(func(info *HubInfo) {
        execute(c, info)
      })
    })

    manners.ListenAndServe(":51700",  router)
    //router.Run(":51700")
  }

  signal_chan := make(chan os.Signal)
  signal.Notify(signal_chan, syscall.SIGTERM)
  go func() {
      for {
          s := <-signal_chan
          if s == syscall.SIGTERM {
              manners.Close()
          }
      }
  }()
}

