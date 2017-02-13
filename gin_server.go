package main

import (
  "gopkg.in/gin-gonic/gin.v1"
  "net/http"
  "fmt"
  "os"
  "io"
  "io/ioutil"
)

type UploadedFile struct {
  Name string
  Time string//time.Time
}

type Node struct {
  Id int
  Name string
  Addr string
  Status int
}

const dateTimeLayout = "2006-01-02 15:04:05"

var nodes = make(map[string]Node)

func index(c *gin.Context) {
  files, _ := ioutil.ReadDir("./files")
  size := len(files)
  lists := make([]UploadedFile, size)
  for i := 0; i < size; i++ {
    lists[i] = UploadedFile{ files[i].Name(), files[i].ModTime().Format(dateTimeLayout) }
  }
  c.HTML(http.StatusOK, "index.tmpl", gin.H {
    "files": lists,
  })
}

func execute(c *gin.Context) {
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

  index(c)
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

  // initailize
  router := gin.Default()

  // load templates
  router.LoadHTMLGlob("./templates/*")

  // resources
  router.StaticFile("/script.js", "./resources/script.js")

  // root 
  router.GET("/", index)

  // loaders
  router.POST("/upload", upload)
  router.GET("/download/:file", download)

  // execute
  router.POST("/execute", execute)

  router.Run(":8080")
}

