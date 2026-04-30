package handler

import (
	"context"
	"nozomi/internal/logger"

	"maunium.net/go/mautrix/event"
)

// 处理 m.room.member 事件
func (r *Router) HandleMember(ctx context.Context, evt *event.Event) {
	memberEvent := evt.Content.AsMember()
	if memberEvent == nil || evt.StateKey == nil || *evt.StateKey != string(r.cfg.Client.UserID) {
		return
	}

	switch memberEvent.Membership {
	case event.MembershipInvite:
		// 自动同意加入房间
		rooms, err := r.matrix.GetJoinedRooms(ctx)
		if err != nil {
			_ = r.logger.Log("error", "Get joined rooms error: "+err.Error(), logger.Options{})
			return
		}
		for _, room := range rooms {
			if room == evt.RoomID.String() {
				return
			}
		}
		err = r.matrix.JoinRoom(ctx, evt.RoomID)
		if err == nil {
			_ = r.matrix.SendText(ctx, evt.RoomID, "Hello, I am Nozomi.")
			_ = r.logger.Log("info", "Auto accept room invite: "+evt.RoomID.String(), logger.Options{})
		}
	case event.MembershipLeave, event.MembershipBan:
		// 被踢出或退出时，物理清除该房间的所有记忆
		r.memory.Delete(evt.RoomID.String())
		_ = r.logger.Log("info", "Auto clear memory of didn't join room: "+evt.RoomID.String(), logger.Options{})
	}
}
