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
  "time"
  "encoding/json"
  "math/rand"
  "path/filepath"
  "bufio"
  "sort"
)

const (
  temporaryBackupFile = "xht_autobackup.txt"
  descriptionFile = "xhub_descriptions.json"
  dateTimeSimple = "20060102150405"
  dateTimeLayout = "2006-01-02 15:04:05"
  dateTimeTemplateLayout = "20060102_150405"
)

type UploadedFile struct {
  Name string
  Time string
  Description string
  TimeInt int
}

type AssignPriority struct {
  Priority int
  // 0: None(initial)
  // 1: Primary
  // 2: Secondary
  Domain *Domain
  ServiceServer *ServiceServer
}

type Domain struct {
  Key string
  Class string

  AssignPriorities []*AssignPriority
}

type ServiceServer struct {
  Port string
  Status int
  // 0: Stopped(initial)
  // 1: Active
  // 2: Synchronizing
  // 8: Warning
  // 9: Danger
  Module string

  LastModifiedAt time.Time
  Name string
  Node *Node
  AssignPriorities []*AssignPriority
}

type Node struct {
  IP string
  Status int
  // 0: Stopped(initial)
  // 1: Active
  // 8: Warning
  // 9: Danger
  Message string

  LastModifiedAt time.Time
  Name string
  ServiceServers map[string]*ServiceServer
}

type HubInfo struct {
  Template string
  Nodes map[string]*Node
  Domains map[string]*Domain
  Descriptions map[string]string
}

func Restore(templateName string, filePath string) (info *HubInfo, err error) {
  info = &HubInfo {
    Template: templateName,
    Nodes: map[string]*Node{},
    Domains: map[string]*Domain{},
  }

  fp, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
  if err != nil { return }
  defer fp.Close()

  reader := bufio.NewReader(fp)
  var isPrefix bool  = false
  var line []byte
  for err == nil {
    line, isPrefix, err = reader.ReadLine()
    if isPrefix {
      var subline []byte
      for isPrefix {
        subline, isPrefix, err = reader.ReadLine()
        line = append(line, subline...)
      }
    }
    record := string(line)
    parts := strings.Split(record, ">")
    if strings.HasPrefix(record, "N") {// node
      info.Nodes[parts[1]] = &(Node {
        IP: parts[1],
        Status: 0,
        ServiceServers: map[string]*ServiceServer{},
      })
    } else if strings.HasPrefix(record, "S") {// server
      node, has := info.Nodes[parts[1]]
      if has {
        name := ""
        if 3 < len(parts) {
          _, err := os.Stat(filepath.Join("files", parts[3]))
          if !os.IsNotExist(err) {
            name = parts[3]
          }
        }
        node.ServiceServers[parts[2]] = &(ServiceServer {
          Port: parts[2],
          Status: 0,
          Module: name,
          Node: node,
        })
      }
    } else if strings.HasPrefix(record, "D") {// domain
      info.Domains[parts[1]] = &(Domain {
        Key: parts[1],
        Class: "d" + time.Now().Format(dateTimeTemplateLayout) + strconv.Itoa(len(info.Domains)),
      })
    } else if strings.HasPrefix(record, "A") {// assign
      // A>127.0.0.1>:12345>domain.test>1
      node, has := info.Nodes[parts[1]]
      priority, err := strconv.Atoi(parts[4])
      if has && err == nil && 0 < priority && priority < 3 {
        server, has := node.ServiceServers[parts[2]]
        if has {
          domain, has := info.Domains[parts[3]]
          if has {
            assign := &(AssignPriority {
              Priority: priority,
              Domain: domain,
              ServiceServer: server,
            })
            server.AssignPriorities = append(server.AssignPriorities, assign)
            domain.AssignPriorities = append(domain.AssignPriorities, assign)
          }
        }
      }
    } else if strings.HasPrefix(record, "[") {// server name
      // [127.0.0.1>:12345]test
      li := strings.Index(record, "]")
      ipport := record[1:li]
      ip := strings.Split(ipport, ">")[0]
      port := strings.Split(ipport, ">")[1]
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has {
          server.Name = record[(li + 1):]
        }
      }
    }
  }
  err = nil
  return
}

func Backup(info *HubInfo) []byte {
  buf := make([]byte, 0)
  for _, domain := range info.Domains {
    buf = append(buf, ("D>" + domain.Key + "\n")...)
  }
  for _, node := range info.Nodes {
    buf = append(buf, ("N>" + node.IP + "\n")...)
    for _, server := range node.ServiceServers {
      line := "S>" + node.IP + ">" + server.Port
      if "" != server.Module {
        line = line + ">" + server.Module
      }
      line = line + "\n"
      buf = append(buf, line...)
      for index := range server.AssignPriorities {
        assign := server.AssignPriorities[index]
        priority := strconv.Itoa(assign.Priority)
        buf = append(buf, ("A>" + node.IP + ">" + server.Port + ">" + assign.Domain.Key + ">" + priority + "\n")...)
      }
      if "" != server.Name {
        line = "[" + node.IP + ">" + server.Port + "]" + server.Name + "\n"
        buf = append(buf, line...)
      }

    }
  }
  return buf
}

func (node Node) SendUDP(port string, message string) {
  remote, err := net.ResolveUDPAddr("udp", node.IP + port)
  if err != nil { return }
  conn, err := net.DialUDP("udp", nil, remote)
  if err != nil { return }
  conn.SetDeadline(time.Now().Add(3 * time.Second))
  defer conn.Close()
  conn.Write([]byte(message))
}
func (node Node) SendMessage(message string) {
  node.SendUDP(":51710", message)
}

func index(c *gin.Context, info *HubInfo) {
  files, _ := ioutil.ReadDir("files")
  size := len(files)
  lists := make([]UploadedFile, size - 1)
  for i := 0; i < size; i++ {
    if descriptionFile != files[i].Name() {
      val, _ := strconv.Atoi(files[i].ModTime().Format(dateTimeSimple))
      lists[i] = UploadedFile{
        Name: files[i].Name(),
        Time: files[i].ModTime().Format(dateTimeLayout),
        Description: info.Descriptions[files[i].Name()],
        TimeInt: val,
      }
    }
  }
  sort.Slice(lists, func(i, j int) bool {
    return lists[i].TimeInt > lists[j].TimeInt
  })
  rval, err := c.Cookie("reload")
  if err == nil && "" != rval {
    c.SetCookie("reload", "", 10, "/", "", false, true)
  } else {
    for _, node := range info.Nodes {
      for _, server := range node.ServiceServers {
        if server.Status == 2 {
          rval = "3000"
          break
        }
      }
      if "" != rval { break }
    }
  }
  c.HTML(http.StatusOK, "index.tmpl", gin.H {
    "files": lists,
    "template": info.Template,
    "nodes": info.Nodes,
    "domains": info.Domains,
    "reload": rval,
  })
}

func execute(c *gin.Context, info *HubInfo) {
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
      fileName := json["name"].(string)
      os.Remove(filepath.Join("files", fileName))
      delete(info.Descriptions, fileName)
      saveDescriptions(info)
    case "stopNode":
      ip := json["ip"].(string)
      node, has := info.Nodes[ip]
      if has {
        node.SendMessage("C>")
      }
    case "addServer":
      ip := json["ip"].(string)
      node, has := info.Nodes[ip]
      if has {
        node.SendMessage("S>")
      }
    case "renameServer":
      ip := json["ip"].(string)
      port := json["port"].(string)
      name := json["name"].(string)
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has {
          server.Name = name
        }
      }
    case "startServer":
      ip := json["ip"].(string)
      port := json["port"].(string)
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has && 0 == server.Status {
          server.Status = 2
          server.LastModifiedAt = time.Now()
          node.SendMessage("S>" + port)
        }
      }
    case "stopServer":
      ip := json["ip"].(string)
      port := json["port"].(string)
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has && 1 == server.Status {
          server.Status = 2
          node.SendMessage("C>" + port)
        }
      }
    case "syncServer":
      ip := json["ip"].(string)
      port := json["port"].(string)
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has && (1 == server.Status || 8 == server.Status) && "" != server.Module {
          _, err := os.Stat(filepath.Join("files", server.Module))
          if !os.IsNotExist(err) {
            server.Status = 2
            node.SendMessage("S>" + port + "><" + server.Module)
          }
        }
      }
    case "setModule":
      name := json["name"].(string)
      _, err := os.Stat(filepath.Join("files", name))
      if !os.IsNotExist(err) {
        ip := json["ip"].(string)
        port := json["port"].(string)
        node, has := info.Nodes[ip]
        if has {
          server, has := node.ServiceServers[port]
          if has && 1 == server.Status && server.Module != name {
            server.Module = name
            server.Status = 2
            node.SendMessage("S>" + port + ">" + server.Module)
          }
        }
      }
    case "addDomain":
      newDomain := json["name"].(string)
      info.Domains[newDomain] = &(Domain {
        Key: newDomain,
        Class: "d" + time.Now().Format(dateTimeTemplateLayout),
      })
    case "delDomain":
      delDomain := json["name"].(string)
      domain, has := info.Domains[delDomain]
      if has {
        for index := range domain.AssignPriorities {
          assign := domain.AssignPriorities[index]
          if nil != assign.ServiceServer {
            priorities := make([]*AssignPriority, 0)
            for i := range assign.ServiceServer.AssignPriorities {
              a := assign.ServiceServer.AssignPriorities[i]
              if a != assign {
                priorities = append(priorities, a)
              }
            }
            assign.ServiceServer.AssignPriorities = priorities
          }
        }
        delete(info.Domains, delDomain)
      }
    case "assign":
      ip := json["ip"].(string)
      port := json["port"].(string)
      domainKey := json["domain"].(string)
      priority, err := strconv.Atoi(json["priority"].(string))
      node, has := info.Nodes[ip]
      if has && nil == err {
        server, has := node.ServiceServers[port]
        if has {
          unique := true
          for i := range server.AssignPriorities {
            if domainKey == server.AssignPriorities[i].Domain.Key {
              server.AssignPriorities[i].Priority = priority
              unique = false
              break
            }
          }
          if unique {
            domain, has := info.Domains[domainKey]
            if has {
              assign := AssignPriority {
                Priority: priority,
                Domain: domain,
                ServiceServer: server,
              }
              server.AssignPriorities = append(server.AssignPriorities, &assign)
              domain.AssignPriorities = append(domain.AssignPriorities, &assign)
            }
          }
        }
      }
    case "exclude":
      ip := json["ip"].(string)
      port := json["port"].(string)
      domainKey := json["domain"].(string)
      node, has := info.Nodes[ip]
      if has {
        server, has := node.ServiceServers[port]
        if has {
          assigns := make([]*AssignPriority, 0)
          for index := range server.AssignPriorities {
            assign := server.AssignPriorities[index]
            if domainKey != assign.Domain.Key {
              assigns = append(assigns, assign)
            }
          }
          server.AssignPriorities = assigns
        }
      }
      domain, has := info.Domains[domainKey]
      if has {
        assigns := make([]*AssignPriority, 0)
        for index := range domain.AssignPriorities {
          assign := domain.AssignPriorities[index]
          if port != assign.ServiceServer.Port {
            assigns = append(assigns, assign)
          }
        }
        domain.AssignPriorities = assigns
      }
    default:
  }
  index(c, info)
  //c.Redirect(http.StatusMovedPermanently, "/")
}
func saveDescriptions(info *HubInfo) {
  bytes, err := json.Marshal(info.Descriptions)
  if err == nil {
    ioutil.WriteFile(filepath.Join("files", descriptionFile), bytes, 0644)
  } 
}
func upload(c *gin.Context, caller chan *HubInfo) {
  file, header, err := c.Request.FormFile("file")
  if err == nil {
    info := <- caller

    // get filename
    fileName := header.Filename
    filePath := filepath.Join("files", fileName)
    description := c.Request.FormValue("description")
    descSave := "" != description
    if "on" == c.Request.FormValue("backup") {
      _, err := os.Stat(filePath)
      if !os.IsNotExist(err) {
        backupName := time.Now().Format(dateTimeTemplateLayout)  + "_" + fileName
        os.Rename(filePath, filepath.Join("files", backupName))
        info.Descriptions[backupName] = info.Descriptions[fileName]
        descSave = true
      }
    }
    if descSave {
      info.Descriptions[fileName] = description
      saveDescriptions(info)
    }
    // create file
    out, err := os.Create(filePath)
    if err == nil {
      defer out.Close()
      // copy from temporary
      _, err = io.Copy(out, file)
      if err == nil && "template" ==  c.Request.FormValue("key") {
        newInfo, err := Restore(fileName, filePath)
        if err == nil {
          newInfo.Descriptions = info.Descriptions
          os.Remove(filePath)
          c.SetCookie("reload", "3000", 10, "/", "", false, true)
          unlock(caller, newInfo)
        } else {
          unlock(caller, info)
        }
      } else {
        unlock(caller, info)
      }
    } else {
      unlock(caller, info)
    }
  }
  if err != nil {
    fmt.Printf("Error: %s\n", err)
  }
  c.Redirect(http.StatusMovedPermanently, "/")
}

func download(c *gin.Context, info *HubInfo) {
  fileName := c.Param("file")

  var bytes []byte
  if fileName == "template" {
    bytes = Backup(info)
    fileName = "xht_" + time.Now().Format(dateTimeTemplateLayout) + ".txt"
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

func exchange(caller chan *HubInfo, info *HubInfo) {
fmt.Println(1)
  <- caller
fmt.Println(2)
  caller <- info
}

func unlock(caller chan *HubInfo, info *HubInfo) {
  caller <- info
}

func lock(caller chan *HubInfo, fn func(*HubInfo)) {
  info := <- caller
  defer unlock(caller, info)
  for _, node := range info.Nodes {
    if 0 != node.Status && 9 != node.Status {
      seconds := int(time.Since(node.LastModifiedAt).Seconds())
      if 30 < seconds {
        node.Status = 9
      } else if 15 < seconds {
        node.Status = 8
      }
    }
    for _, server := range node.ServiceServers {
      if 0 != server.Status && 9 != server.Status {
        seconds := int(time.Since(server.LastModifiedAt).Seconds())
        if 30 < seconds {
          server.Status = 9
        } else if 15 < seconds {
          server.Status = 8
        }
      }
    }
  }
  fn(info)
  ioutil.WriteFile(temporaryBackupFile, Backup(info), 0644)
}

func loadDescriptions() map[string]string {
  filePath := filepath.Join("files", descriptionFile)
  _, err := os.Stat(filePath)
  if os.IsNotExist(err) {
    os.Create(filePath)
  }
  blob, err := ioutil.ReadFile(filePath)
  var descriptions map[string]string
  json.Unmarshal(blob, &descriptions)
  if descriptions == nil {
    descriptions = map[string]string {}
  }
  return descriptions
}

func main() {
  // create files directory.
  _, err := os.Stat("files")
  if err != nil {
    os.Mkdir("files", 0755)
  }

  // description
  descriptions := loadDescriptions()

  // resource channel.
  cInfo := make(chan *HubInfo)

  {// ResourceMaster
    go func() {
      var info *HubInfo
      empty := true
      _, err = os.Stat(temporaryBackupFile)
      if !os.IsNotExist(err) { 
        tempInfo, err := Restore("Auto backup", temporaryBackupFile)
        if err == nil {
          info = tempInfo
          empty = false
        }
      }
      if empty {
        info = &HubInfo {
          Template: "",
          Nodes: map[string]*Node{},
          Domains: map[string]*Domain{},
        }
      }
      info.Descriptions = descriptions
      for {
        cInfo <- info
        info = <- cInfo
      }
    }()
  }

  {// CommunicationServer
    fmt.Println("UDP START!!")
    addr, err := net.ResolveUDPAddr("udp", ":51701")
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
          fmt.Printf("Receive %v:%v -> %v\n", remote.IP, remote.Port, message)
          if strings.HasPrefix(message, "D") {
            // D>Domain
            targets := strings.Split(message, "@")
            if 1 < len(targets) {
              target := targets[1]
              lock(cInfo, func(info *HubInfo) {
                err := true
                domain, has := info.Domains[target]
                if !has {
                  length := 0
                  for key, dom := range info.Domains {
                    if 0 == strings.Index(target, key) {
                      if length < len(key) {
                        domain = dom
                        length = len(key)
                        err = false
                      }
                    }
                  }
                } else {
                  err = false
                }
                var server *ServiceServer
                if !err {
                  primaries := make([]*ServiceServer, 0)
                  secondaries := make([]*ServiceServer, 0)
                  for index := range domain.AssignPriorities {
                    assign := domain.AssignPriorities[index]
                    if 1 == assign.ServiceServer.Node.Status && 1 == assign.ServiceServer.Status {
                      if 1 == assign.Priority {
                        primaries = append(primaries, assign.ServiceServer)
                      } else if 2 == assign.Priority {
                        secondaries = append(secondaries, assign.ServiceServer)
                      }
                    }
                  }
                  rand.Seed(time.Now().UnixNano())
                  if 0 < len(primaries) {
                    server = primaries[rand.Intn(len(primaries))]
                  } else if 0 < len(secondaries) {
                    server = secondaries[rand.Intn(len(secondaries))]
                  } else {
                    err = true
                  }
                }
                if err {
                  fmt.Printf("%v\n", err)
                  conn.WriteToUDP([]byte("E@NotAssignDomain"), remote)
                } else {
                  fmt.Println("D@" + server.Node.IP + server.Port)
                  conn.WriteToUDP([]byte("D@" + server.Node.IP + server.Port), remote)
                }
              })
            } else {
              conn.WriteToUDP([]byte("E@NotAssignDomain"), remote)
            }
          } else if strings.HasPrefix(message, "C") {
            // C[>PortNo]
            parts := strings.Split(message, ">")
            lock(cInfo, func(info *HubInfo) {
              node, has := info.Nodes[remote.IP.String()]
              if has {
                if 1 == len(parts) {
                  // node stop
                  node.Status = 0
                  for _, server := range node.ServiceServers {
                    server.Status = 0
                  }
                } else {
                  port := parts[1] 
                  server, has := node.ServiceServers[port]
                  if has {
                    // server stop
                    server.Status = 0
                  }
                }
              }
            })
          } else if strings.HasPrefix(message, "N") {
            // N[>PortNo][>Module]
            parts := strings.Split(message, ">")
            lock(cInfo, func(info *HubInfo) {
              node, has := info.Nodes[remote.IP.String()]
              if !has {
                node = &(Node {
                  IP: remote.IP.String(),
                  Status: 1,
                  ServiceServers: map[string]*ServiceServer{},
                })
                info.Nodes[node.IP] = node
              }
              node.Status = 1
              node.LastModifiedAt = time.Now()
              if 1 < len(parts) {
                port := parts[1] 
                server, has := node.ServiceServers[port]
                if !has {
                  moduleName := ""
                  if 2 < len(parts) {
                    _, err := os.Stat(filepath.Join("files", parts[2]))
                    if !os.IsNotExist(err) {
                      moduleName = parts[2]
                    }
                  }
                  server = &(ServiceServer {
                    Port: port,
                    Status: 1,
                    Module: moduleName,
                    Node: node,
                  })
                  node.ServiceServers[server.Port] = server
                }
                server.Status = 1
                server.LastModifiedAt = time.Now()
                if "" != server.Module {
                  if 2 < len(parts) {
                    if server.Module != parts[2] {
                      server.Status = 2
                      node.SendMessage("S>" + server.Port + ">" + server.Module)
                    }
                  } else {
                    server.Status = 2
                    node.SendMessage("S>" + server.Port + ">" + server.Module)
                  }
                } else if 2 < len(parts) {
                  server.Module = parts[2]
                }
              }
            })
          } else if strings.HasPrefix(message, "E") {
            // E@Message
            parts := strings.Split(message, "@")
            fmt.Printf("  %s\n", parts[1])
          }
          // ignore othres.
        }
      }
    }()
  }

  {// WebServer
    // initailize
    router := gin.Default()
    // load templates
    router.LoadHTMLGlob("./templates/*")
    // root
    //router.GET("/", index)
    router.GET("/", func(c *gin.Context) {
      lock(cInfo, func(info *HubInfo) {
        index(c, info)
      })
    })
    // loaders
    router.POST("/upload", func(c *gin.Context) {
      upload(c, cInfo)
    })
    router.GET("/download/:file", func(c *gin.Context) {
      lock(cInfo, func(info *HubInfo) {
        download(c, info)
      })
    })
    // execute
    router.POST("/execute", func(c *gin.Context) {
      lock(cInfo, func(info *HubInfo) {
        execute(c, info)
      })
    })
    // resources
    router.StaticFile("/fonts/glyphicons-halflings-regular.woff2", "./resources/glyphicons-halflings-regular.woff2")
    router.StaticFile("/fonts/glyphicons-halflings-regular.woff", "./resources/glyphicons-halflings-regular.woff")
    router.StaticFile("/fonts/glyphicons-halflings-regular.ttf", "./resources/glyphicons-halflings-regular.ttf")
    router.StaticFile("/fonts/glyphicons-halflings-regular.svg", "./resources/glyphicons-halflings-regular.svg")
    router.StaticFile("/fonts/glyphicons-halflings-regular.eot", "./resources/glyphicons-halflings-regular.eot")
    router.StaticFile("/bootstrap-theme.min.css", "./resources/bootstrap-theme.min.css")
    router.StaticFile("/bootstrap-theme.min.css.map", "./resources/bootstrap-theme.min.css.map")
    router.StaticFile("/bootstrap.min.css", "./resources/bootstrap.min.css")
    router.StaticFile("/bootstrap.min.css.map", "./resources/bootstrap.min.css.map")
    router.StaticFile("/bootstrap.min.js", "./resources/bootstrap.min.js")
    router.StaticFile("/jquery.min.js", "./resources/jquery.min.js")
    router.StaticFile("/jquery.min.map", "./resources/jquery.min.map")
    router.StaticFile("/script.js", "./resources/script.js")

    // listen
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

