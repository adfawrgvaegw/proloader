// Package map_art generates Minecraft Bedrock map art by converting an image
// into a mosaic of blocks with carefully chosen heights to reproduce the map
// palette's brightness variations.
//
// This package is designed to write blocks directly into a LevelDB-based
// Bedrock world via bedrock-world-operator's BedrockWorld.
package map_art
