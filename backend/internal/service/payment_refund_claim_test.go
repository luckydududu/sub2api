//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// claimRefundDeduction 的行锁完全依赖「ent 的裸 Update 也会因 updated_at 的
// UpdateDefault 而发出真实 UPDATE」这一行为。如果哪天该行为变了(或有人以为这句
// 没有 setter 的 Update 是空操作而删掉它),锁就会静默失效,陈旧锁接管与原执行的
// 并发窗口重新打开、可能双扣——而其它测试一个都不会红。
//
// 这个用例就是钉住它:调用后订单行的 updated_at 必须真的前进。
func TestClaimRefundDeductionTakesRealRowWrite(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("claim-guard@example.com").
		SetPasswordHash("x").
		SetUsername("claim-guard-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("CLAIM-GUARD-ORDER").
		SetOutTradeNo("sub2_claim_guard_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-claim-guard").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	before, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)

	// sqlite 的时间戳精度有限,先等一小会儿以便 updated_at 的推进可观测。
	time.Sleep(10 * time.Millisecond)

	proceed, err := svc.claimRefundDeduction(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, proceed, "尚无守卫审计行时应当放行扣减")

	after, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Truef(t, after.UpdatedAt.After(before.UpdatedAt),
		"claimRefundDeduction 必须对订单行产生真实写入以取得行锁;updated_at 未前进(before=%s after=%s)说明该 UPDATE 被优化掉了,行锁已静默失效",
		before.UpdatedAt, after.UpdatedAt)

	// 守卫审计行落库后,复查必须拦下第二次扣减。
	require.NoError(t, svc.writeAuditLogErr(ctx, order.ID, refundFinalDeductionAuditAction, "admin", map[string]any{"balanceDeducted": 50}))

	proceed, err = svc.claimRefundDeduction(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, proceed, "已存在 REFUND_FINAL_DEDUCTION_DONE 时必须拦下重复扣减")
}
