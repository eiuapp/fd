// Package fd provides a simple API to pass file descriptors
// between different OS processes.
//
// It can be useful if you want to inherit network connections
// from another process without closing them.
//
// Example scenario:
// - Running server receives a "let's upgrade" message
// - Server opens a Unix domain socket for the "upgrade"
// - Server starts a new copy of itself and passes Unix domain
//   socket name
// - New copy starts reading for the socket
// - Server sends its state over the socket, also sending the number
//   of network connections to inherit, then it sends those connections
//   using fd.Put()
// - New copy reads the state and inherits connections using fd.Get(),
//   checks that everything is OK and sends the "OK" message to the socket
// - Server receives "OK" message and kills itself
package fd

import (
	"net"
	"os"
	"syscall"
)

// Get receives file descriptors over Unix domain socket.
//
// Num specifies the expected number of file descriptors in one message.
// Internal files' names to be assigned are specified via optional filenames
// argument.
//
// Use net.FileConn() if you're receiving a network connection. Don't
// forget to close the returned *os.File though.
func Get(via *net.UnixConn, num int, filenames []string) ([]*os.File, error) {
	if num < 1 {
		return nil, nil
	}

	// get the underlying socket
	viaf, err := via.File()
	if err != nil {
		return nil, err
	}
	socket := int(viaf.Fd())
	defer viaf.Close()

	// recvmsg
	buf := make([]byte, syscall.CmsgSpace(num*4))
	_, _, _, _, err = syscall.Recvmsg(socket, nil, buf, 0)
	if err != nil {
		return nil, err
	}

	// parse control msgs
	var msgs []syscall.SocketControlMessage
	msgs, err = syscall.ParseSocketControlMessage(buf)

	// convert fds to files
	res := make([]*os.File, 0, len(msgs))
	for i := 0; i < len(msgs) && err == nil; i++ {
		var fds []int
		fds, err = syscall.ParseUnixRights(&msgs[i])

		for fi, fd := range fds {
			var filename string
			if fi < len(filenames) {
				filename = filenames[fi]
			}

			res = append(res, os.NewFile(uintptr(fd), filename))
		}
	}

	return res, err
}

// Put file descriptors into Unix domain socket.
//
// Please note that the number of descriptors in one message is limited
// and is rather small.
// Use conn.File() to get a file if you want to put a network connection.
func Put(via *net.UnixConn, files ...*os.File) error {
	if len(files) == 0 {
		return nil
	}

	viaf, err := via.File()
	if err != nil {
		return err
	}
	socket := int(viaf.Fd())
	defer viaf.Close()

	fds := make([]int, len(files))
	for i := range files {
		fds[i] = int(files[i].Fd())
	}

	rights := syscall.UnixRights(fds...)
	return syscall.Sendmsg(socket, nil, rights, nil, 0)
}