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
  _ "strconv"
  "time"
  "encoding/json"
)

type UploadedFile struct {
  Name string
  Time string//time.Time
}

type ServiceServer struct {
  Port string
  Status int
  Module string

  Name string
  Node Node
}

type Node struct {
  IP string
  Port string

  Name string
  ServiceServers []ServiceServer
}

type Domain struct {
  Key string
  Class string

  ServiceServers []ServiceServer
}

type HubInfo struct {
  Template string
  Nodes []Node
  Domains map[string]Domain
}

const (
  dateTimeLayout = "2006-01-02 15:04:05"
  dateTimeTemplateLayout = "20060102_150405"
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
    "template": h.Template,
    "nodes": h.Nodes,
    "domains": h.Domains,
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
    case "removeFile":
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

func download(c *gin.Context, h *HubInfo) {
  fileName := c.Param("file") // c.PostForm("name")

  var bytes []byte
  if fileName == "template" {
    bytes, _ = json.Marshal(h)
    fileName = "xht_" + time.Now().Format(dateTimeTemplateLayout) + ".json"
  } else {
    file, err := os.Open("./files/" + fileName)
    if err != nil {
      fmt.Printf("Error: %s\n", err)
      c.Status(http.StatusNotFound)
      return
    }
    defer file.Close()

    bytes, err = ioutil.ReadAll(file)
    if err != nil {
      fmt.Printf("Error: %s\n", err)
      c.Status(http.StatusNotFound)
      return
    }
  }

  c.Header("Content-Disposition", "attachment; filename=" + fileName )
  c.Data(http.StatusOK, "application/zip", bytes)
}

func main() {
  // create files directory.
  _, err := os.Stat("files")
  if err != nil {
    os.Mkdir("files", 0755)
  }

  // resource channel.
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

      info.Template = "test_template.json"
      // :> debug
      node0 := Node {
        IP: "192.168.0.1",
        Port: ":51710",
        Name: "node0",
      }
      test0 := ServiceServer {
        Port: ":51711",
        Status: 0,
        Module: "20170221_test_v1.0.0.jar",
        Node: node0,
      }
      node0.ServiceServers = append(node0.ServiceServers, test0)

      node1 := Node {
        IP: "192.168.0.2",
        Port: ":51710",
        Name: "node1",
      }
      test1 := ServiceServer {
        Port: ":51711",
        Status: 1,
        Module: "20170221_test_v1.0.1.jar",
        Name: "",
        Node: node1,
      }
      test2 := ServiceServer {
        Port: ":51712",
        Status: 2,
        Module: "20170221_test_v1.0.2.jar",
        Name: "test server name",
        Node: node1,
      }
      node1.ServiceServers = append(node1.ServiceServers, test1)
      node1.ServiceServers = append(node1.ServiceServers, test2)

      info.Nodes = append(info.Nodes, node0)
      info.Nodes = append(info.Nodes, node1)

      info.Domains = map[string]Domain {}
      info.Domains["domain0"] = Domain {
        Key: "domain0",
        Class: "domain0",
        ServiceServers: []ServiceServer{ test0 },
      }
      info.Domains["domain1"] = Domain {
        Key: "domain1",
        Class: "domain1",
        ServiceServers: []ServiceServer{ test0, test1 },
      }
      info.Domains["domain2"] = Domain {
        Key: "domain2",
        Class: "domain2",
        ServiceServers: []ServiceServer{ test0, test1, test2 },
      }
      info.Domains["domain3"] = Domain {
        Key: "domain3",
        Class: "domain3",
        ServiceServers: []ServiceServer{ test1, test2 },
      }
      // <: debug

      for {
        cInfo <- info
        info = <- cInfo
        //select {
        //  case cInfo <- info:
        //    info = <- cInfo
        //}
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
              //status, _ := strconv.Atoi(parts[1])
              info.Nodes = append(info.Nodes, Node {
                Port: parts[1],
                Name: parts[2],
                IP: parts[3],
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
        index(c, info)
      })
   })

    // loaders
    router.POST("/upload", upload)
    router.GET("/download/:file", func(c *gin.Context) {
      fInfo(func(info *HubInfo) {
        download(c, info)
      })
    })
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

