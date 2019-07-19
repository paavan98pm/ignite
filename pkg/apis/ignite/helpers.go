package ignite

import (
	"path"

	"github.com/weaveworks/ignite/pkg/constants"
)

// GetNetworkModes gets the list of available network modes
func GetNetworkModes() []NetworkMode {
	return []NetworkMode{
		NetworkModeCNI,
		NetworkModeDockerBridge,
	}
}

// GetImageSourceTypes gets the list of available network modes
func GetImageSourceTypes() []ImageSourceType {
	return []ImageSourceType{
		ImageSourceTypeDocker,
	}
}

// GetVMStates gets the list of available VM states
func GetVMStates() []VMState {
	return []VMState{
		VMStateCreated,
		VMStateRunning,
		VMStateStopped,
	}
}

// SetImage populates relevant fields to an Image on the VM object
func (vm *VM) SetImage(image *Image) {
	vm.Spec.Image.OCIClaim = image.Spec.OCIClaim
	vm.Status.Image = image.Status.OCISource
}

// SetKernel populates relevant fields to a Kernel on the VM object
func (vm *VM) SetKernel(kernel *Kernel) {
	vm.Spec.Kernel.OCIClaim = kernel.Spec.OCIClaim
	vm.Status.Kernel = kernel.Status.OCISource
}

// SnapshotDev returns the path where the (legacy) DM snapshot exists
func (vm *VM) SnapshotDev() string {
	// TODO: Reuse the prefixer here
	return path.Join("/dev/mapper", constants.IGNITE_PREFIX+vm.GetUID().String())
}

// Running returns true if the VM is running, otherwise false
func (vm *VM) Running() bool {
	return vm.Status.State == VMStateRunning
}

// OverlayFile returns the path to the overlay.dm file for the VM.
// TODO: This will be removed once we have the new snapshotter in place.
func (vm *VM) OverlayFile() string {
	return path.Join(vm.ObjectPath(), constants.OVERLAY_FILE)
}

// ObjectPath returns the directory where this VM's data is stored
func (vm *VM) ObjectPath() string {
	// TODO: Move this into storage
	return path.Join(constants.DATA_DIR, vm.GetKind().Lower(), vm.GetUID().String())
}

// ObjectPath returns the directory where this Image's data is stored
func (img *Image) ObjectPath() string {
	// TODO: Move this into storage
	return path.Join(constants.DATA_DIR, img.GetKind().Lower(), img.GetUID().String())
}

// ObjectPath returns the directory where this Kernel's data is stored
func (k *Kernel) ObjectPath() string {
	// TODO: Move this into storage
	return path.Join(constants.DATA_DIR, k.GetKind().Lower(), k.GetUID().String())
}
