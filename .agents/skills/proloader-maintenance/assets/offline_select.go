// Local analysis helper; not part of the upstream snapshot.
package mcpk
import (
 "fmt"
 "github.com/Yeah114/unmcpk/pymarshal"
)
func (fs *MCPKFileSystem) AuditDecode(dirID, fileID uint32) ([]byte,error) {
 if !fs.prepareFileEntries(dirID) { return nil,fmt.Errorf("directory absent") }
 for _,e:=range fs.FileEntries[dirID] {
  if e.Id!=fileID {continue}
  a:=uint64(fs.offsetFileEntryEnd)+uint64(e.Offset);z:=a+uint64(e.Size)
  if z>uint64(len(fs.data)) {return nil,fmt.Errorf("entry outside carrier")}
  raw,err:=decrypt(fs.data[a:z],e.Id);if err!=nil{return nil,err}
  return pymarshal.PycRepairer(raw,e.Id!=idRedirect)
 }
 return nil,fmt.Errorf("file absent")
}
