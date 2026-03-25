package bot

import (
	"context"
	"fmt"

	"nozomi/internal/logger"

	"maunium.net/go/mautrix/event"
)

func AutoAcceptInvite(ctx context.Context, evt *event.Event) {
	memberEvent := evt.Content.AsMember()
	if memberEvent == nil {
		return
	}

	if memberEvent.Membership == event.MembershipInvite {
		if evt.StateKey != nil && *evt.StateKey == client.UserID.String() {
			joinedRoomsResp, err := client.JoinedRooms(ctx)
			if err != nil {
				_ = logger.Log("error", "Failed to get the joined room list."+err.Error(), logger.Options{})
				return
			}
			for _, roomID := range joinedRoomsResp.JoinedRooms {
				if roomID == evt.RoomID {
					return
				}
			}
			_ = logger.Log("info", fmt.Sprintf("Receive a room Invite from %s with room ID: %s", evt.Sender, evt.RoomID), logger.Options{})
			_, err = client.JoinRoomByID(ctx, evt.RoomID)
			if err != nil {
				str := "Failed to join room with ID: " + evt.RoomID.String()
				_ = logger.Log("error", str, logger.Options{})
			} else {
				_ = logger.Log("info", fmt.Sprintf("Join room(%s) sucessfully.", evt.RoomID), logger.Options{})
				_, _ = client.SendText(ctx, evt.RoomID, "你好，我是希。")
			}
		}
	}
}
