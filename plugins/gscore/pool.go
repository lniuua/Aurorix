package gscore

import "sync"

// 下行投递池：把「读 socket」与「发 QQ」解耦。
//
// 读循环只负责快速把 gscore 下行帧入队，真正发消息（图片要 base64 解码 + 上传 QQ）
// 由投递池异步完成，慢速的媒体上传不再阻塞 socket 读取。
//
// 并发模型：
//   - 按目标会话分片（shard），同一会话内严格保序（busy 标志保证单在途）；
//   - 不同会话之间可并行（受 deliverConcurrency 信号量约束）；
//   - 空队列的 shard 会及时回收，不会随会话数无限增长。
const (
	// deliverConcurrency 同时进行的投递上限，防止对 QQ 侧打得太猛。
	deliverConcurrency = 16
	// deliverShardCap 单个会话的待投递队列上限；超限丢最旧保最新（安全阀，正常不会触发）。
	deliverShardCap = 512
)

type deliverShard struct {
	queue  []MessageSend
	busy   bool // 该 shard 是否有在途投递
	queued bool // 该 shard 是否已在 pending 队列中，避免重复入队
}

type deliverPool struct {
	mu      sync.Mutex
	shards  map[string]*deliverShard
	pending []string // 有待投递消息且不在途的 shard key
	sem     chan struct{}
}

var deliverer = &deliverPool{
	shards: make(map[string]*deliverShard),
	sem:    make(chan struct{}, deliverConcurrency),
}

func shardKey(send *MessageSend) string {
	t := send.TargetType
	if t == "" {
		t = "group"
	}
	if t == "group" {
		return "g:" + send.TargetID
	}
	return "p:" + send.TargetID
}

// backlog 返回当前待投递的消息条数（不含在途那条），供控制台展示积压情况。
func (p *deliverPool) backlog() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, sh := range p.shards {
		n += len(sh.queue)
	}
	return n
}

// submit 入队一条下行消息。只做「查 shard + 追加」，O(1) 且不阻塞 socket 读取。
func (p *deliverPool) submit(send *MessageSend) {
	key := shardKey(send)
	p.mu.Lock()
	sh := p.shards[key]
	if sh == nil {
		sh = &deliverShard{}
		p.shards[key] = sh
	}
	if len(sh.queue) >= deliverShardCap {
		// 安全阀：该会话积压过多，丢最旧、保最新。
		adapterLog.Warnf("gscore 投递队列积压超过 %d，丢弃最旧消息 (target=%s)", deliverShardCap, key)
		sh.queue = append(sh.queue[:0], sh.queue[1:]...)
	}
	sh.queue = append(sh.queue, *send)
	if !sh.busy && !sh.queued {
		sh.queued = true
		p.pending = append(p.pending, key)
	}
	p.mu.Unlock()

	p.pump()
}

// pump 尝试启动一次投递：从 pending 中挑一个可投递的 shard。
func (p *deliverPool) pump() {
	select {
	case p.sem <- struct{}{}:
	default:
		// 并发已满；正在运行的投递完成后会再次 pump。
		return
	}

	p.mu.Lock()
	var key string
	var sh *deliverShard
	var send MessageSend
	found := false
	for len(p.pending) > 0 {
		key = p.pending[0]
		p.pending = p.pending[1:]
		sh = p.shards[key]
		if sh == nil {
			continue
		}
		sh.queued = false
		if sh.busy || len(sh.queue) == 0 {
			continue
		}
		sh.busy = true
		send = sh.queue[0]
		sh.queue = sh.queue[1:]
		found = true
		break
	}

	if !found {
		p.mu.Unlock()
		<-p.sem
		return
	}
	p.mu.Unlock()

	go p.deliver(key, sh, send)
}

func (p *deliverPool) deliver(key string, sh *deliverShard, send MessageSend) {
	deliverSend(&send)

	p.mu.Lock()
	sh.busy = false
	if len(sh.queue) > 0 {
		if !sh.queued {
			sh.queued = true
			p.pending = append(p.pending, key)
		}
	} else if p.shards[key] == sh {
		// 空队列且仍是同一个 shard 才回收，避免误删并发新建的 shard。
		delete(p.shards, key)
	}
	p.mu.Unlock()

	<-p.sem
	p.pump()
}
