// Package neteaseworld implements the XOR-based encryption/decryption used by
// some NetEase Minecraft Bedrock world LevelDB directories.
//
// File format (as inferred from NeteaseMC.py in this repo):
//   - Encrypted files start with a fixed 4-byte header: 0x80 0x1d 0x30 0x01.
//   - The remaining bytes are XOR'ed with a key, repeating the key as needed.
//   - The key is derived from the encrypted CURRENT file and the manifest file
//     name: key = XOR(CURRENT_body, manifestName+"\n").
//
// Encryption is symmetric (XOR), so decryption uses the same operation.
package netease_world
