package main
import("os";"fmt";"path/filepath";"github.com/Yeah114/unmcpk/mcpk")
func main(){
 if len(os.Args)!=3{panic("input and absent output directory required")}
 b,e:=os.ReadFile(os.Args[1]);if e!=nil{panic(e)}
 fs,e:=mcpk.NewMCPKFileSystem(b);if e!=nil{panic(e)}
 if e=os.Mkdir(os.Args[2],0700);e!=nil{panic(e)}
 for _,ids:=range [][2]uint32{{0x8bbfa2c7,0xd6367fb9},{0x94b92985,0xd94c8a8c},{0x94b92985,0x12e0e8fc}}{
  data,e:=fs.AuditDecode(ids[0],ids[1]);if e!=nil{panic(e)}
  name:=fmt.Sprintf("%x_%x.pyc",ids[0],ids[1]);p:=filepath.Join(os.Args[2],name)
  f,e:=os.OpenFile(p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{panic(e)}
  _,e=f.Write(data);f.Close();if e!=nil{panic(e)};fmt.Println(name,len(data))
 }
}
