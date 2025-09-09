package main

import (
	"fmt"
	"net/http"
	"src/apis"
	"src/database"
	"context"
)

func main(){
	//Init DB
	redisCli := database.LoadRedis()
	ctx := context.Background()
	redisCli.FlushAll(ctx)

	//Rest API
	mux := http.NewServeMux()
	api.Routes(mux,redisCli)
	fmt.Println("Listening on port 8080")
	http.ListenAndServe(":8080",mux)
}