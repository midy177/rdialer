package main

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"net"
	"net/http"
	"rdialer"
	"strconv"
	"time"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	client, err := rdialer.NewClient("https://192.168.12.40:8443/connect")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create NewClient")
	}
	header := make(http.Header)
	header.Set("tunnel-id", "1234")
	go func() {
		for {
			err = client.Connect(context.Background(), header)
			if err != nil {
				log.Error().Err(err).Msg("Failed to connect to rdialer,after 10s retry")
			}
			client.Close()
			time.Sleep(10 * time.Second)
		}
	}()

	// 创建自定义传输层
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 获取自定义 Dialer
			dialer, err := client.GetDialer()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get remote dialer")
			}
			return dialer(ctx, network, addr)
		},
	}
	e := echo.New()
	e.GET("/:scheme/:host", func(c echo.Context) error {
		return Client(transport, c)
	})

	log.Fatal().Err(e.Start(":1323")).Send()
}

func Client(transport *http.Transport, c echo.Context) error {
	rw := c.Response().Writer
	timeout := c.QueryParam("timeout")
	if timeout == "" {
		timeout = "15"
	}
	scheme := c.Param("scheme")
	host := c.Param("host")
	url := fmt.Sprintf("%s://%s%s", scheme, host, "")
	client := getClient(transport, timeout)

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, h := range v {
			if c.Request().Header.Get(k) == "" {
				c.Response().Header().Set(k, h)
			} else {
				c.Response().Header().Add(k, h)
			}
		}
	}
	rw.WriteHeader(resp.StatusCode)
	_, err = io.Copy(rw, resp.Body)
	return err
}

func getClient(transport *http.Transport, timeout string) *http.Client {
	client := &http.Client{
		Transport: transport,
	}
	if timeout != "" {
		t, err := strconv.Atoi(timeout)
		if err == nil {
			client.Timeout = time.Duration(t) * time.Second
		}
	}
	return client
}
