package item_stack_transaction

import "github.com/Yeah114/FunShuttler/game_control/resources_control"

// responseMapping ..
type responseMapping struct {
	mapping resources_control.ItemStackResponseMapping
}

// newResponseMapping ..
func newResponseMapping() *responseMapping {
	return &responseMapping{mapping: make(resources_control.ItemStackResponseMapping)}
}

// bind ..
func (r *responseMapping) bind(
	windowID resources_control.WindowID,
	containerID resources_control.ContainerID,
) {
	r.mapping[containerID] = windowID
}
