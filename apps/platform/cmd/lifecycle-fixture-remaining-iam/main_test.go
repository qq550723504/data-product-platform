package main

import (
	"net/http"
	"testing"
)

type shouldNotCall struct{t *testing.T}
func(s shouldNotCall)RoundTrip(*http.Request)(*http.Response,error){s.t.Fatal("forbidden bootstrap operation reached transport");return nil,nil}
func TestBootstrapNeverExposesObjectDeletionOrUnconfirmedEnable(t *testing.T){
	for _,tc:=range []struct{method,path string}{
		{"DELETE","/dpp-lifecycle307-online/fixture/p1.csv?versionId=exact"},
		{"PUT","/minio/admin/v3/set-user-status?accessKey=foreign&status=disabled"},
		{"PUT","/minio/admin/v3/set-user-status?accessKey=delete808a6960&status=enabled"},
		{"PUT","/minio/admin/v3/add-user"},{"GET","/dpp-lifecycle307-online/fixture/p1.csv?versionId=exact"},
	}{t.Run(tc.path,func(t *testing.T){req,_:=http.NewRequest(tc.method,"http://127.0.0.1:9000"+tc.path,nil);if _,e:=(transport{shouldNotCall{t},map[string]string{"delete808a6960":"fixed"},true}).RoundTrip(req);e==nil{t.Fatal("forbidden request admitted")}})}
}
