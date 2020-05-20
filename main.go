package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// postman json
type ApiItem struct{
	Name string `json:"name"`
	Request interface{} `json:"request"`
}

type ApiJsonInfo struct {
	Info struct{
		PostmanId string `json:"_postman_id"`
		Name string `json:"name"`
		Schema string `json:"schema"`
	} `json:"info"`
	Item []ApiItem `json:"item"`
}

type Resp struct{
	Error bool `json:"error"`
	Msg string `json:"msg"`
}

type ApiJson struct{
	Version string `json:"version"`
	Path string `json:"path"`
	Timestamp string `json:"timestamp"`
}

func getVersions() []string{
	cmd := exec.Command("bash", "-c", "ls -alt ./json | grep api | awk -F 'api_' '{print $2}' | sed 's/.json//g'")
	stdout, _ := cmd.Output()
	versions := strings.Split(string(stdout), "\n")
	versions = versions[:len(versions)-1]
	return versions
}

func generateResp(msg string, error bool)string{
	b, _ := json.Marshal(Resp{
		Error: error,
		Msg: msg,
	})
	return string(b)
}

func list(w http.ResponseWriter, _ *http.Request){
	versions := getVersions()
	var jsonList []ApiJson
	for _, value := range versions{
		f := "/json/api_"+value+".json"
		finfo, err := os.Stat("." + f)
		if err != nil{
			fmt.Println(err)
			continue
		}
		jsonList = append(jsonList, ApiJson{
			Version: value,
			Path: f,
			Timestamp: finfo.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	b, err := json.Marshal(jsonList)
	if err != nil{
		fmt.Println(err)
		io.WriteString(w, generateResp("json failed", true))
		return
	}
	io.WriteString(w, string(b))
}

func upload(w http.ResponseWriter, r *http.Request){
	r.ParseMultipartForm(10 << 20)
	force := r.FormValue("force")
	fmt.Println(force)
	file, _, err := r.FormFile("apiJsonFile")
	defer file.Close()
	if err != nil {
		fmt.Println("Error Retrieving the File")
		fmt.Println(err)
		io.WriteString(w, generateResp("upload failed", true))
		return
	}
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println(err)
	}
	versions := getVersions()
	maxVersion, _ := strconv.Atoi(versions[0])
	if force == "false"{
		// 取出最近一个版本和当前版本比较内容
		info := new(ApiJsonInfo)
		err = json.Unmarshal(fileBytes, info)
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json parse failed", true))
			return
		}
		// TODO:检查版本号是否匹配

		// 获取老版本的
		maxVersionContent, _ := ioutil.ReadFile("./json/api_"+strconv.Itoa(maxVersion)+".json")
		oldInfo := new(ApiJsonInfo)
		err = json.Unmarshal(maxVersionContent, oldInfo)
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json parse failed", true))
			return
		}
		oldLen := len(oldInfo.Item)

		// 将新的APIS转成hash，方便去重
		unq := make(map[string]bool)
		for _, value := range oldInfo.Item{
			unq[value.Name] = true
		}
		for _, value := range info.Item{
			if unq[value.Name]{
				continue
			}
			oldInfo.Item = append(oldInfo.Item, value)
		}
		if oldLen == len(oldInfo.Item){
			io.WriteString(w, generateResp("no new api found, please upload again", true))
			return
		}
		// 再json化
		b, err := json.Marshal(oldInfo)
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json failed", true))
			return
		}
		ioutil.WriteFile("./json/api_"+strconv.Itoa(maxVersion+1)+".json", b, 0644)
	}else{
		ioutil.WriteFile("./json/api_"+strconv.Itoa(maxVersion+1)+".json", fileBytes, 0644)
	}

	io.WriteString(w, generateResp("success", false))
}

func main(){
	runtime.GOMAXPROCS(runtime.NumCPU() - 1)
	http.HandleFunc("/list", list)
	http.HandleFunc("/upload", upload)
	http.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir("./json"))))
	http.Handle("/", http.FileServer(http.Dir("./template")))
	http.ListenAndServe(":3001", nil)
}
