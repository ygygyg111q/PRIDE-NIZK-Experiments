package main

import (
	"PRIDE-Exp/Util"
	"errors"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto/bn256"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"
)

type Commitment struct {
	TildeV    bn256.G1
	TildeA    bn256.G1
	Timestamp int64
}

type Session struct {
	Commitments map[int64]Commitment
	PiV         bn256.G1
	PiA         bn256.G1
	Started     bool
}

type Cloud struct {
	Session map[uint64]Session
}

//{"jsonrpc": "2.0", "method": "Cloud.Time", "params": [], "id": 1234}
func (this *Cloud) Time(arg Util.RpcNullArgument, res *Util.RpcTimeResponse) error {
	log.Println("Time")
	res.Timestamp = time.Now().Unix()
	return nil
}

//{"jsonrpc": "2.0", "method": "Cloud.NewSession", "params": [{"CarID":12345}], "id": 12345}
func (this *Cloud) NewSession(arg Util.RpcCarIDArgument, res *Util.RpcNullResponse) (er error) {
	log.Println("[NewSession] Incoming")
	//catch panic and throw as an error
	defer func(ret *error) {
		if err := recover(); err != nil {
			*ret = errors.New("[CloudFatal] " + fmt.Sprint(err))
		}
	}(&er)

	if arg.CarID == 0 {
		return errors.New("CarID is required.")
	}
	session, existed := this.Session[arg.CarID]
	if existed {
		if session.Started {
			return errors.New("Session already started.")
		}
	}

	session = Session{}

	session.Started = true
	session.Commitments = make(map[int64]Commitment, 0)

	//注意，为了规避 bn256 库的一个 bug（大概是 bug，已经提交 PR，等开发者回复），
	//需要让这个点自己加自己一次。反正是零元（单位元），不影响结果。
	session.PiV = Util.NewG1IdenticalElement()
	session.PiA = Util.NewG1IdenticalElement()

	//var p bn256.G1
	//var x_bytes, y_bytes []byte
	//
	//p.Unmarshal(append(x_bytes[:], y_bytes[:]...))
	//session.PiV.Unmarshal( make([]byte, 64))
	//session.PiA.Unmarshal( make([]byte, 64))

	this.Session[arg.CarID] = session
	log.Println("[NewSession] carID = " + fmt.Sprint(arg.CarID))

	return nil

}

//{"jsonrpc": "2.0", "method": "Cloud.Commit", "params": [{"CarID":12345,"Timestamp":2,"TildeV":[4929377010393555783522434351721163191462599000300542932299803988670102596561, 7981972865453785282492749202557663332964667188560053463769866091745624846725],"TildeA":[1,2]}], "id": 12345}
func (this *Cloud) Commit(arg Util.RpcCommitArgument, res *Util.RpcNullResponse) (er error) {
	log.Println("[Commit] Incoming")
	//catch panic and throw as an error
	defer func(ret *error) {
		if err := recover(); err != nil {
			*ret = errors.New("[CloudFatal] " + fmt.Sprint(err))
		}
	}(&er)

	if arg.Timestamp == 0 {
		return errors.New("timestamp is required")
	}

	if arg.CarID == 0 {
		return errors.New("CarID is required")
	}

	session, existed := this.Session[arg.CarID]
	if !existed {
		return errors.New("session is not started")
	}

	if !session.Started {
		return errors.New("session is not started")
	}

	_, existed = session.Commitments[arg.Timestamp]
	if existed {
		return errors.New("the commitment of the given timestamp already exist")
	}

	commitment := Commitment{}

	//save TildeV
	commitment.TildeV, er = Util.StringXYToG1(arg.TildeV)
	if er != nil {
		return er
	}
	//save TildeA
	commitment.TildeA, er = Util.StringXYToG1(arg.TildeA)
	if er != nil {
		return er
	}

	//save timestamp
	commitment.Timestamp = arg.Timestamp

	//update Pi_V and Pi_A
	session.PiV.Add(&session.PiV, &commitment.TildeV)
	session.PiA.Add(&session.PiA, &commitment.TildeA)

	//save commitment
	session.Commitments[arg.Timestamp] = commitment
	log.Println("[Commit] time=" + fmt.Sprint(commitment.Timestamp) + ", car=" + fmt.Sprint(arg.CarID))
	return nil
}

func (this *Cloud) Sign(arg Util.RpcSignArgument, res *Util.RpcSignResponse) (er error) {
	log.Println("[Sign] Incoming")
	//catch panic and throw as an error
	defer func(ret *error) {
		if err := recover(); err != nil {
			*ret = errors.New("[CloudFatal] " + fmt.Sprint(err))
		}
	}(&er)

	if arg.CarID == 0 {
		return errors.New("CarID is required")
	}

	session, existed := this.Session[arg.CarID]
	if !existed {
		return errors.New("session is not started")
	}

	if !session.Started {
		return errors.New("session is not started")
	}

	piV, err := Util.StringXYToG1(arg.PiV)
	if err != nil {
		return err
	}

	//通过比较封送处理的结果(X,Y)来判断两点是否相等
	if !Util.G1Equals(piV, session.PiV) {
		return errors.New("PiV is not equal")
	}

	piA, err := Util.StringXYToG1(arg.PiA)
	if err != nil {
		return err
	}
	if !Util.G1Equals(piA, session.PiA) {
		return errors.New("PiA is not equal")
	}

	//用 OK 代替真的签名，因为这不是重点
	res.Signature = "OK"

	log.Println("[Sign] " + res.Signature)

	return nil
}

func registerRpc() error {
	cloud := new(Cloud)
	cloud.Session = make(map[uint64]Session)
	return rpc.Register(cloud)
}

func listenAndServeJsonRpc(port uint16) {

	addr, _ := net.ResolveTCPAddr("tcp", ":"+fmt.Sprint(port))
	ln, e := net.ListenTCP("tcp", addr)
	if e != nil {
		log.Panic(e)
	}

	log.Println("Listening on TCP port " + fmt.Sprint(port) + "...")
	for {
		conn, e := ln.Accept()
		if e != nil {
			continue
		}
		go jsonrpc.ServeConn(conn)
	}
}

func main() {
	port := flag.Int("port", 0, "(required)")
	flag.Parse()

	if !(*port <= 65535 && *port >= 1) {
		flag.Usage()
		return
	}

	err := registerRpc()
	if err != nil {
		log.Panic(err)
	}
	listenAndServeJsonRpc(uint16(*port))
}
