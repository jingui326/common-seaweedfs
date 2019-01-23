package sub

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/pubsub"
	"github.com/chrislusf/seaweedfs/weed/glog"
	"github.com/chrislusf/seaweedfs/weed/pb/filer_pb"
	"github.com/chrislusf/seaweedfs/weed/util"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/option"
)

func init() {
	NotificationInputs = append(NotificationInputs, &GooglePubSubInput{})
}

type GooglePubSubInput struct {
	sub         *pubsub.Subscription
	topicName   string
	messageChan chan *pubsub.Message
}

func (k *GooglePubSubInput) GetName() string {
	return "google_pub_sub"
}

func (k *GooglePubSubInput) Initialize(configuration util.Configuration) error {
	glog.V(0).Infof("notification.google_pub_sub.project_id: %v", configuration.GetString("project_id"))
	glog.V(0).Infof("notification.google_pub_sub.topic: %v", configuration.GetString("topic"))
	return k.initialize(
		configuration.GetString("google_application_credentials"),
		configuration.GetString("project_id"),
		configuration.GetString("topic"),
	)
}

func (k *GooglePubSubInput) initialize(google_application_credentials, projectId, topicName string) (err error) {

	ctx := context.Background()
	// Creates a client.
	if google_application_credentials == "" {
		var found bool
		google_application_credentials, found = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
		util.LogFatalIf(!found, "need to specific GOOGLE_APPLICATION_CREDENTIALS env variable or google_application_credentials in filer.toml")
	}

	client, err := pubsub.NewClient(ctx, projectId, option.WithCredentialsFile(google_application_credentials))
	util.LogFatalIfError(err, "Failed to create client: %v", err)

	k.topicName = topicName
	topic := client.Topic(topicName)
	exists, err := topic.Exists(ctx)
	util.LogFatalIfError(err, "Failed to check topic %s: %v", topicName, err)
	if !exists {
		topic, err = client.CreateTopic(ctx, topicName)
		util.LogFatalIfError(err, "Failed to create topic %s: %v", topicName, err)
	}

	subscriptionName := "seaweedfs_sub"

	k.sub = client.Subscription(subscriptionName)
	exists, err = k.sub.Exists(ctx)
	util.LogFatalIfError(err, "Failed to check subscription %s: %v", topicName, err)

	if !exists {
		k.sub, err = client.CreateSubscription(ctx, subscriptionName, pubsub.SubscriptionConfig{Topic: topic})
		util.LogFatalIfError(err, "Failed to create subscription %s: %v", subscriptionName, err)
	}

	k.messageChan = make(chan *pubsub.Message, 1)

	go k.sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		k.messageChan <- m
		m.Ack()
	})

	return err
}

func (k *GooglePubSubInput) ReceiveMessage() (key string, message *filer_pb.EventNotification, err error) {

	m := <-k.messageChan

	// process the message
	key = m.Attributes["key"]
	message = &filer_pb.EventNotification{}
	err = proto.Unmarshal(m.Data, message)

	if err != nil {
		err = fmt.Errorf("unmarshal message from google pubsub %s: %v", k.topicName, err)
		return
	}

	return
}
