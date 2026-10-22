package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "flag"
 "io"
 "os"
 "sort"
 "time"
 "github.com/minio/minio-go/v7"
 "github.com/minio/minio-go/v7/pkg/credentials"
)
type config struct { RootKey,RootSecret string; Profile struct {StorageEndpoint,Bucket string} }
type object struct {Key string `json:"key"`; VersionID string `json:"versionId"`; Size int64 `json:"size"`; Sha256 string `json:"sha256"`}
func main(){
 endpoint:=flag.String("endpoint","","isolated restored endpoint override")
 output:=flag.String("output","","manifest path")
 flag.Parse()
 raw,err:=os.ReadFile(os.Getenv("TEST_INGESTION_B3_CONFIG")); must(err)
 var cfg config;must(json.Unmarshal(raw,&cfg))
 if *endpoint!="" {cfg.Profile.StorageEndpoint=*endpoint}
 client,err:=minio.New(cfg.Profile.StorageEndpoint,&minio.Options{Creds:credentials.NewStaticV4(cfg.RootKey,cfg.RootSecret,""),Secure:false});must(err)
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 result:=[]object{}
 for entry:=range client.ListObjects(ctx,cfg.Profile.Bucket,minio.ListObjectsOptions{Recursive:true,WithVersions:true}) {
  must(entry.Err)
  if entry.IsDeleteMarker {panic("unexpected delete marker")}
  stream,err:=client.GetObject(ctx,cfg.Profile.Bucket,entry.Key,minio.GetObjectOptions{VersionID:entry.VersionID});must(err)
  hash:=sha256.New();size,err:=io.Copy(hash,stream);must(err);must(stream.Close())
  if size!=entry.Size {panic("size mismatch")}
  result=append(result,object{entry.Key,entry.VersionID,size,hex.EncodeToString(hash.Sum(nil))})
 }
 sort.Slice(result,func(i,j int)bool{return result[i].Key+result[i].VersionID<result[j].Key+result[j].VersionID})
 raw,err=json.MarshalIndent(result,"","  ");must(err);must(os.WriteFile(*output,raw,0600))
}
func must(err error){if err!=nil {panic(err)}}
