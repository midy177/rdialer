package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"rdialer"
	"sync"
	"time"
)

func main() {
	client, err := rdialer.NewClient("https://127.0.0.1:8443/connect")
	if err != nil {
		log.Fatal(err)
	}
	header := make(http.Header)
	header.Set("tunnel-id", "1234")
	err = client.Connect(context.Background(), header)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	// 获取自定义 Dialer
	dialer, err := client.GetDialer()
	if err != nil {
		log.Fatal(err)
	}
	// 创建自定义传输层
	transport := &http.Transport{
		DialContext: dialer,
	}
	//httpGet(transport)
	wg := &sync.WaitGroup{}
	sg := make(chan struct{}, 10)
	for i := 0; i < 1000; i++ {
		time.Sleep(1 * time.Second)
		wg.Add(1)
		sg <- struct{}{}
		go func() {
			httpGet(transport)
			<-sg
			wg.Done()
		}()
	}
	time.Sleep(3 * time.Second)
}

func httpGet(transport *http.Transport) {
	// 创建 HTTP 客户端
	httpClient := &http.Client{
		Transport: transport,
	}

	// 发起 HTTP GET 请求
	resp, err := httpClient.Get("http://baidu.com")
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("响应状态码: %d\n", resp.StatusCode)
	log.Printf("响应内容: %s\n", string(body))
}
