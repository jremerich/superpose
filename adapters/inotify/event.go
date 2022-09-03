//go:build linux

package inotify

import (
	"fmt"
	"strings"
	"syscall"
)

const (
	InAccess       = uint32(syscall.IN_ACCESS)        // File was accessed
	InAttrib       = uint32(syscall.IN_ATTRIB)        // Metadata changed
	InCloseWrite   = uint32(syscall.IN_CLOSE_WRITE)   // File opened for writing was closed.
	InCloseNowrite = uint32(syscall.IN_CLOSE_NOWRITE) // File or directory not opened for writing was closed.
	InCreate       = uint32(syscall.IN_CREATE)        // File/directory created in watched directory
	InDelete       = uint32(syscall.IN_DELETE)        // File/directory deleted from watched directory.
	InDeleteSelf   = uint32(syscall.IN_DELETE_SELF)   // Watched file/directory was itself deleted.
	InModify       = uint32(syscall.IN_MODIFY)        // File was modified
	InMoveSelf     = uint32(syscall.IN_MOVE_SELF)     // Watched file/directory was itself moved.
	InMovedFrom    = uint32(syscall.IN_MOVED_FROM)    // Generated for the directory containing the old filename when a file is renamed.
	InMovedTo      = uint32(syscall.IN_MOVED_TO)      // Generated for the directory containing the new filename when a file is renamed.
	InOpen         = uint32(syscall.IN_OPEN)          // File or directory was opened.

	InAllEvents = uint32(syscall.IN_ALL_EVENTS) // bit mask of all of the above events.
	InMove      = uint32(syscall.IN_MOVE)       // Equates to InMovedFrom | InMovedTo.
	InClose     = uint32(syscall.IN_CLOSE)      // Equates to InCloseWrite | InCloseNowrite.

	/* The following further bits can be specified in mask when calling Inotify.addWatch() */

	InDontFollow = uint32(syscall.IN_DONT_FOLLOW) // Don't dereference pathname if it is a symbolic link.
	InExclUnlink = uint32(syscall.IN_EXCL_UNLINK) // Don't generate events for children if they have been unlinked from the directory.
	InMaskAdd    = uint32(syscall.IN_MASK_ADD)    // Add (OR) the events in mask to the watch mask
	InOneshot    = uint32(syscall.IN_ONESHOT)     // Monitor the filesystem object corresponding to pathname for one event, then remove from watch list.
	InOnlydir    = uint32(syscall.IN_ONLYDIR)     // Watch pathname only if it is a directory.

	/* The following bits may be set in the mask field returned by Inotify.read() */

	InIgnored   = uint32(syscall.IN_IGNORED)    // Watch was removed explicitly or automatically
	InIsDir     = uint32(syscall.IN_ISDIR)      // Subject of this event is a directory.
	InQOverflow = uint32(syscall.IN_Q_OVERFLOW) // Event queue overflowed (wd is -1 for this event).

	InUnmount = uint32(syscall.IN_UNMOUNT) // Filesystem containing watched object was unmounted.
)

var in_mapping = map[uint32]string{
	InAccess:       "IN_ACCESS",
	InAttrib:       "IN_ATTRIB",
	InCloseWrite:   "IN_CLOSE_WRITE",
	InCloseNowrite: "IN_CLOSE_NOWRITE",
	InCreate:       "IN_CREATE",
	InDelete:       "IN_DELETE",
	InDeleteSelf:   "IN_DELETE_SELF",
	InModify:       "IN_MODIFY",
	InMoveSelf:     "IN_MOVE_SELF",
	InMovedFrom:    "IN_MOVED_FROM",
	InMovedTo:      "IN_MOVED_TO",
	InMove:         "IN_MOVE",
	InOpen:         "IN_OPEN",
	InIgnored:      "IN_IGNORED",
	InIsDir:        "IN_ISDIR",
	InQOverflow:    "IN_Q_OVERFLOW",
	InUnmount:      "IN_UNMOUNT",
}

func InMaskToString(in_mask uint32) string {
	sb := &strings.Builder{}
	divide := false
	for mask, str := range in_mapping {
		if in_mask&mask == mask {
			if divide {
				sb.WriteString("|")
			}
			sb.WriteString(str)
			divide = true
		}
	}
	return sb.String()
}

func InStringToMask(in_mask string) uint32 {
	var inMask uint32

	mapIn := strings.Split(in_mask, "|")
	for _, strMask := range mapIn {
		strMask = strings.TrimSpace(strMask)
		for mask, str := range in_mapping {
			if str == strMask {
				inMask += mask
				break
			}
		}
	}

	return inMask
}

// InotifyEvent is the go representation of inotify_event found in sys/inotify.h
type InotifyEvent struct {
	// Watch descriptor
	Wd uint32
	// File or directory name
	Name string
	// Contains bits that describe the event that occurred
	Mask uint32
	// Usually 0, but if events (like InMovedFrom and InMovedTo) are linked then they will have equal cookie
	Cookie uint32
}

func (i InotifyEvent) GoString() string {
	return fmt.Sprintf("gonotify.InotifyEvent{Wd=%#v, Name=%s, Cookie=%#v, Mask=%#v=%s", i.Wd, i.Name, i.Cookie, i.Mask, InMaskToString(i.Mask))
}

func (i InotifyEvent) String() string {
	return fmt.Sprintf("{Wd=%d, Name=%s, Cookie=%d, Mask=%s", i.Wd, i.Name, i.Cookie, InMaskToString(i.Mask))
}

func (i InotifyEvent) Is(needle uint32) bool {
	return i.Mask&needle == needle
}

func (i InotifyEvent) IsSyncEvent() bool {
	return i.Is(InCloseWrite) || i.Is(InDelete) || i.Is(InDeleteSelf) || i.Is(InMove)
}

// FileEvent is the wrapper around InotifyEvent with additional Eof marker. Reading from
// FileEvents from DirWatcher.C or FileWatcher.C may end with Eof when underlying inotify is closed
type FileEvent struct {
	InotifyEvent
	Eof bool
}
