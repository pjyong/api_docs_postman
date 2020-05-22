package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	apiServerUrl string = "https://shopping.yrxsy.com/"
	apiServerUrlParts = []string{"shopping", "yrxsy", "com"}
)

// postman json
type UrlInfo struct {
	Raw string `json:"raw"`
	Protocol string `json:"protocol"`
	Host []string `json:"host"`
	Path []string `json:"path"`
	Query interface{} `json:"query"`
	Port string `json:"port"`
}

type RequestInfo struct {
	Method string `json:"method"`
	Header interface{} `json:"header"`
	Body interface{} `json:"body"`
	Url UrlInfo `json:"url"`
	Description string `json:"description"`
}

type ApiItem struct{
	Name string `json:"name"`
	Request RequestInfo `json:"request"`
	Response interface{} `json:"response"`
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
		p := "/json?f=api_"+value
		f := "/json/api_"+value+".json"
		finfo, err := os.Stat("." + f)
		if err != nil{
			fmt.Println(err)
			continue
		}
		jsonList = append(jsonList, ApiJson{
			Version: value,
			Path: p,
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

func get(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["f"]
	if !ok || len(keys[0]) < 1 {
		io.WriteString(w, "Url Param 'f' is missing")
		return
	}
	keys2, ok := r.URL.Query()["d"]
	if !ok {
		io.WriteString(w, "Url Param 'd' is missing")
		return
	}
	key := keys[0]
	domain := keys2[0]
	f := "./json/"+key+".json"
	fmt.Println(key)
	fmt.Println(domain)
	content, err := ioutil.ReadFile(f)
	if err != nil{
		io.WriteString(w, "File not found")
		return
	}
	if(domain != ""){
		info := new(ApiJsonInfo)
		err = json.Unmarshal(content, info)
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json parse failed", true))
			return
		}
		// 检测domain带不带port
		domainParts := strings.Split(domain, ":")
		port := ""
		if len(domainParts) > 1{
			port = domainParts[1]
		}
		for index, _ := range info.Item{
			info.Item[index].Request.Url.Host = strings.Split(domainParts[0], ".")
			info.Item[index].Request.Url.Protocol = "http"
			info.Item[index].Request.Url.Port = port
			// 将Raw里面的url替换一下
			re, _ := regexp.Compile("http[s]{0,1}://[0-9a-zA-Z_:\\.]+/");
			info.Item[index].Request.Url.Raw = re.ReplaceAllString(info.Item[index].Request.Url.Raw, "http://"+domain+"/")
		}
		content, err = json.MarshalIndent(info, "", "\t")
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json failed", true))
			return
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment;filename=\""+key+".json\"")
	io.WriteString(w, string(content))
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
	info := new(ApiJsonInfo)
	err = json.Unmarshal(fileBytes, info)
	if err != nil{
		fmt.Println(err)
		io.WriteString(w, generateResp("json parse failed", true))
		return
	}
	// 检查配置的版本信息
	if ok, _ := regexp.Match("v2.1", []byte(info.Info.Schema)); !ok {
		io.WriteString(w, generateResp("json version not matched v2.1", true))
		return
	}
	// 将新的url全部替换成测试环境的
	for index, _ := range info.Item{
		info.Item[index].Request.Url.Host = apiServerUrlParts
		info.Item[index].Request.Url.Protocol = "https"
		info.Item[index].Request.Url.Port = ""
		// 将Raw里面的url替换一下
		re, _ := regexp.Compile("http[s]{0,1}://[0-9a-zA-Z_:\\.]+/");
		info.Item[index].Request.Url.Raw = re.ReplaceAllString(info.Item[index].Request.Url.Raw, apiServerUrl)
	}
	if force == "false"{
		// 取出服务器最近一个版本和当前版本比较内容，合并
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
		b, err := json.MarshalIndent(info, "", "\t")
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json failed", true))
			return
		}
		ioutil.WriteFile("./json/api_"+strconv.Itoa(maxVersion+1)+".json", b, 0644)
	}else{
		// 再json化
		b, err := json.MarshalIndent(info, "", "\t")
		if err != nil{
			fmt.Println(err)
			io.WriteString(w, generateResp("json failed", true))
			return
		}
		ioutil.WriteFile("./json/api_"+strconv.Itoa(maxVersion+1)+".json", b, 0644)
	}

	io.WriteString(w, generateResp("success", false))
}

func main(){
	//fileBytes, _ := ioutil.ReadFile("./json/api_16.json")
	//re, _ := regexp.Compile("http[s]{0,1}://[0-9a-zA-Z_:\\.]+/");
	//all := re.FindAll(fileBytes, -1);
	//for _, value := range all{
	//	fmt.Println(string(value))
	//}
	runtime.GOMAXPROCS(runtime.NumCPU() - 1)
	http.HandleFunc("/list", list)
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/json", get)
	//http.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir("./json"))))
	http.Handle("/", http.FileServer(http.Dir("./template")))
	http.ListenAndServe(":3001", nil)
}
