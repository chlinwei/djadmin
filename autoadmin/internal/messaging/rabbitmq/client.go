package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"autoadmin/internal/job"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Route 描述一条作业队列的完整拓扑（交换机 / 队列 / 路由键 / 死信）。
//
// 平台上有两条队列，区别不在业务而在**执行者需要什么**：
//
//   - JobRoute：只依赖数据库与控制器本身（计划任务、ansible 作业），由 worker 角色消费；
//   - LogCollectRoute：执行时要通过 agent gRPC 会话下发命令（Filebeat 采集配置下发/安装），
//     而 agent 会话只存在于**跑 api 角色的进程**里（agents 连的是 api 的 AGENT_GRPC_ADDRESS，
//     Gateway 是进程内的会话表，见 internal/agent/gateway.go）。所以这条队列必须由 api 角色消费：
//     放到 worker 角色上，handler.gateway.IsOnline() 恒为 false，作业会全部失败。
type Route struct {
	Name               string
	Exchange           string
	Queue              string
	RoutingKey         string
	DeadLetterExchange string
	DeadLetterQueue    string
}

// JobRoute 是不需要 agent 会话的通用作业队列（历史默认队列，worker 角色消费）。
var JobRoute = Route{
	Name:               "job",
	Exchange:           "autoadmin.jobs",
	Queue:              "autoadmin.job.execute",
	RoutingKey:         "job.execute",
	DeadLetterExchange: "autoadmin.jobs.dead",
	DeadLetterQueue:    "autoadmin.job.dead",
}

// LogCollectRoute 是需要 agent 会话的日志采集作业队列（api 角色消费，见 Route 的说明）。
var LogCollectRoute = Route{
	Name:               "logcollect",
	Exchange:           "autoadmin.logcollect",
	Queue:              "autoadmin.logcollect.execute",
	RoutingKey:         "logcollect.execute",
	DeadLetterExchange: "autoadmin.logcollect.dead",
	DeadLetterQueue:    "autoadmin.logcollect.dead",
}

// Routes 是全部队列拓扑，装配方逐条声明。
var Routes = []Route{JobRoute, LogCollectRoute}

type Client struct {
	connection *amqp.Connection
}

func Dial(url string) (*Client, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial RabbitMQ: %w", err)
	}
	return &Client{connection: connection}, nil
}

func (client *Client) Close() error {
	return client.connection.Close()
}

func (client *Client) DeclareTopology() error {
	for _, route := range Routes {
		if err := client.declareRoute(route); err != nil {
			return err
		}
	}
	return nil
}

func (client *Client) declareRoute(route Route) error {
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ topology channel: %w", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(route.DeadLetterExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare %s dead-letter exchange: %w", route.Name, err)
	}
	if _, err := channel.QueueDeclare(route.DeadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare %s dead-letter queue: %w", route.Name, err)
	}
	if err := channel.QueueBind(route.DeadLetterQueue, route.RoutingKey, route.DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind %s dead-letter queue: %w", route.Name, err)
	}
	if err := channel.ExchangeDeclare(route.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare %s exchange: %w", route.Name, err)
	}
	arguments := amqp.Table{"x-dead-letter-exchange": route.DeadLetterExchange, "x-dead-letter-routing-key": route.RoutingKey}
	if _, err := channel.QueueDeclare(route.Queue, true, false, false, false, arguments); err != nil {
		return fmt.Errorf("declare %s queue: %w", route.Name, err)
	}
	if err := channel.QueueBind(route.Queue, route.RoutingKey, route.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind %s queue: %w", route.Name, err)
	}
	return nil
}

// Publish 投递到默认作业队列（满足 scheduler.Publisher）。
func (client *Client) Publish(ctx context.Context, message job.Message) error {
	return client.PublishVia(ctx, JobRoute, message)
}

// PublishLogCollect 投递到日志采集批量作业队列（需要 agent 会话的作业，由 api 角色消费）。
func (client *Client) PublishLogCollect(ctx context.Context, message job.Message) error {
	return client.PublishVia(ctx, LogCollectRoute, message)
}

// PublishVia 按指定路由投递作业消息。
func (client *Client) PublishVia(ctx context.Context, route Route, message job.Message) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode job message: %w", err)
	}
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ publish channel: %w", err)
	}
	defer channel.Close()

	return channel.PublishWithContext(ctx, route.Exchange, route.RoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    message.ExecutionID,
		Body:         body,
	})
}

type Handler interface {
	Handle(context.Context, job.Message) error
}

// Consumer 是一个自带队列拓扑与消费参数的作业消费者：角色装配方在进程启动时拉起它。
//
// StartReaper 是可选的失联对账（执行者消失后收敛停摆的作业）——它单独成方法是因为
// 对账要能在没有消息在途时也周期性地跑，且进程退出时要随 ctx 一起停。
type Consumer interface {
	Consume(ctx context.Context, client *Client) error
	StartReaper(ctx context.Context)
}

// Consume 消费默认作业队列（worker 角色）。
func (client *Client) Consume(ctx context.Context, consumer string, prefetch int, handler Handler) error {
	return client.ConsumeVia(ctx, JobRoute, consumer, prefetch, handler)
}

// ConsumeVia 以 prefetch 为并发上限消费指定队列。
//
// 这里必须真的并发处理：Handle 在多数作业上会阻塞数分钟（ansible 安装、逐台下发），
// 而 prefetch 的语义是"允许未确认的消息数"——之前这里是 `for delivery := range deliveries`
// 里同步调用 Handle，prefetch 只让消息被取到本地，处理仍然严格串行，同一个队列上
// 后到的消息要等前一个跑完（实测 4 条 prefetch 对吞吐毫无帮助）。
// 改成 prefetch 个 goroutine 后，prefetch 既是未确认上限也是并发上限，
// "有界并发"这一条才落到实处（见 docs/plans/LOG_COLLECTION_LIFECYCLE.md §8 规模基线）。
//
// 并发安全性：amqp091 的 Channel 对 Ack/Nack 内部加锁（channel.go 的 ch.m），
// 各 worker 只做 Ack/Nack（不再碰 Channel 的其它方法），因此可以共用一条 channel。
// 消息之间的互斥由各 handler 自己保证：scheduler 用数据库 Claim，
// 日志批量作业用 monitor_log_batch_job 的 pending→running 认领。
func (client *Client) ConsumeVia(ctx context.Context, route Route, consumer string, prefetch int, handler Handler) error {
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ consumer channel: %w", err)
	}
	defer channel.Close()
	if prefetch < 1 {
		prefetch = 1
	}
	if err := channel.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("set RabbitMQ prefetch: %w", err)
	}
	deliveries, err := channel.ConsumeWithContext(ctx, route.Queue, consumer, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s queue: %w", route.Name, err)
	}
	var workers sync.WaitGroup
	for index := 0; index < prefetch; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for delivery := range deliveries {
				var message job.Message
				if err := json.Unmarshal(delivery.Body, &message); err != nil || message.SchemaVersion != job.SchemaVersion {
					_ = delivery.Nack(false, false)
					continue
				}
				if err := handler.Handle(ctx, message); err != nil {
					// 不重入原队列：Handler 的失败多为"这条消息本身有问题"（未支持的 kind、
					// 资源不存在），立即重投只会形成热循环。死信队列留档，靠失联对账重投。
					slog.Error("handle job message", "route", route.Name, "kind", message.Kind, "resource_id", message.ResourceID, "error", err)
					_ = delivery.Nack(false, false)
					continue
				}
				_ = delivery.Ack(false)
			}
		}()
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	// 连接断开时 deliveries 会被关闭、消费静默结束——如果这里返回 nil，调用方会以为"正常收工"
	// 而不再重试，定时任务与批量作业就再也没人处理。显式报错，让角色装配方按失败处理
	// （systemd 的 Restart=on-failure 会重启进程）。
	return fmt.Errorf("%s queue %s consumer stopped unexpectedly", route.Name, route.Queue)
}
