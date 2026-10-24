// Proloader adapter. The planner below calls Fatalder's unmodified chunk_fill.
// It has no login, socket, or game command execution entry point.
package main

import (
 "encoding/json"
 "encoding/binary"
 "fmt"
 "os"
 "sort"
 "strings"

 "github.com/TriM-Organization/bedrock-world-operator/block"
 "github.com/TriM-Organization/bedrock-world-operator/chunk"
 bwo "github.com/TriM-Organization/bedrock-world-operator/define"
 "github.com/Yeah114/Fatalder/defines"
 "github.com/Yeah114/Fatalder/utils/chunk_fill"
)

type state struct { Name string `json:"name"`; States map[string]any `json:"states"` }
type input struct { Size [3]int `json:"size"`; Origin [3]int `json:"origin"`; Palette []state `json:"palette"`; Blocks []int `json:"blocks"`; BlocksFile string `json:"blocks_file"`; Exact bool `json:"exact_states"` }
type batch struct { Anchor [3]int `json:"anchor"`; Commands []string `json:"commands"` }
func run() error {
 var in input
 if err:=json.NewDecoder(os.Stdin).Decode(&in);err!=nil{return err}
 sx,sy,sz:=in.Size[0],in.Size[1],in.Size[2]
 var raw []byte
 if in.BlocksFile!="" {var err error;raw,err=os.ReadFile(in.BlocksFile);if err!=nil{return err}}
 if sx<1||sy<1||sy>384||sz<1{return fmt.Errorf("invalid dimensions")}
 if in.BlocksFile!="" {if len(raw)!=sx*sy*sz*4{return fmt.Errorf("invalid block file")}} else if sx*sy*sz!=len(in.Blocks){return fmt.Errorf("invalid dimensions")}
 if in.Exact {
  registered:=map[uint32]state{}
  for i,s:=range in.Palette {if s.Name=="minecraft:unknown"||!strings.HasPrefix(s.Name,"minecraft:"){return fmt.Errorf("invalid exact block")};id:=uint32(i+1);if id==block.AirRuntimeID{return fmt.Errorf("runtime collision")};registered[id]=s}
  previous:=block.RuntimeIDToState
  block.RuntimeIDToState=func(id uint32)(string,map[string]any,bool){if s,ok:=registered[id];ok{return s.Name,s.States,true};return previous(id)}
 }
 ids:=make([]uint32,len(in.Palette))
 for i,s:=range in.Palette {
  if s.Name=="minecraft:structure_void"||s.Name=="minecraft:structure_block" {ids[i]=block.AirRuntimeID;continue}
  for k,v:=range s.States {switch x:=v.(type) {case bool: if x{s.States[k]=byte(1)}else{s.States[k]=byte(0)};case float64:s.States[k]=int32(x)}}
  if in.Exact {if s.Name=="minecraft:air"{ids[i]=block.AirRuntimeID}else{ids[i]=uint32(i+1)};continue}
  id,ok:=block.StateToRuntimeID(strings.TrimPrefix(s.Name,"minecraft:"),s.States)
  if !ok {id,ok=block.StateToRuntimeID(s.Name,s.States)}
  if !ok{return fmt.Errorf("unmapped palette state %d: %s",i,s.Name)}
  ids[i]=id
 }
 chunks:=map[defines.ChunkPos]*chunk.Chunk{}
 for x:=0;x<sx;x++ {for y:=0;y<sy;y++ {for z:=0;z<sz;z++ {
  offset:=(x*sy+y)*sz+z;idx:=0;if in.BlocksFile!=""{idx=int(binary.LittleEndian.Uint32(raw[offset*4:]))}else{idx=in.Blocks[offset]};if idx<0||idx>=len(ids){return fmt.Errorf("invalid palette index")};rid:=ids[idx]
  if rid==block.AirRuntimeID{continue}
  pos:=defines.ChunkPos{int32(x/16),int32(z/16)}
  c:=chunks[pos];if c==nil{c=chunk.NewChunk(block.AirRuntimeID,bwo.Range{0,((sy+15)/16)*16-1});chunks[pos]=c}
  c.SetBlock(uint8(x%16),int16(y),uint8(z%16),0,rid)
 }}}
 // Same three-chunk square groups and serpentine row traversal as BuildTask.
 groups:=map[[2]int]map[defines.ChunkPos]*chunk.Chunk{}
 for pos,c:=range chunks {g:=[2]int{int(pos.X())/3,int(pos.Z())/3};if groups[g]==nil{groups[g]=map[defines.ChunkPos]*chunk.Chunk{}};groups[g][pos]=c}
 keys:=make([][2]int,0,len(groups));for g:=range groups{keys=append(keys,g)}
 sort.Slice(keys,func(i,j int)bool{a,b:=keys[i],keys[j];if a[1]!=b[1]{return a[1]<b[1]};if a[1]%2==0{return a[0]<b[0]};return a[0]>b[0]})
 result:=[]batch{}
 for _,g:=range keys {
  minX,minZ:=int32(1<<30),int32(1<<30)
  for p:=range groups[g]{if p.X()<minX{minX=p.X()};if p.Z()<minZ{minZ=p.Z()}}
  start:=defines.BlockPos{int32(in.Origin[0])+minX*16,int32(in.Origin[1]),int32(in.Origin[2])+minZ*16}
  b:=batch{Anchor:[3]int{in.Origin[0]+g[0]*48+23,in.Origin[1],in.Origin[2]+g[1]*48+23},Commands:[]string{}}
  for command:=range chunk_fill.GenerateChunksCommand(groups[g],start){b.Commands=append(b.Commands,strings.TrimSpace(command))}
  result=append(result,b)
 }
 canonical:=make([]state,len(ids))
 for i,id:=range ids {name,props,ok:=block.RuntimeIDToState(id);if !ok{return fmt.Errorf("invalid canonical palette")};if !strings.Contains(name,":"){name="minecraft:"+name};for k,v:=range props{if b,ok:=v.(byte);ok{props[k]=b!=0}};canonical[i]=state{name,props}}
 return json.NewEncoder(os.Stdout).Encode(map[string]any{"batches":result,"palette":canonical})
}
func main(){if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}}
