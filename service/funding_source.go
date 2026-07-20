package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包、订阅或活动权益）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet"、"subscription" 或 "campaign"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

type WalletFunding struct {
	userId    int
	tokenId   int
	requestId string
	consumed  int // 实际预扣的用户额度
	sequence  int
}

// YanCoreEntitlementFunding consumes a matched YanCore campaign entitlement.
// It never falls back to the wallet after a matching entitlement is selected.
type YanCoreEntitlementFunding struct {
	userId        int
	tokenId       int
	requestId     string
	entitlementId int64
	consumed      int
	sequence      int
}

func (f *YanCoreEntitlementFunding) Source() string { return BillingSourceCampaign }

func (f *YanCoreEntitlementFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := f.applyChange(-amount, model.YanCoreEntitlementLedgerReserve, "preconsume", "YanCore campaign entitlement reserve"); err != nil {
		return err
	}
	f.consumed = amount
	return nil
}

func (f *YanCoreEntitlementFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return f.applyChange(-delta, model.YanCoreEntitlementLedgerSettlement, "settlement", "YanCore campaign entitlement settlement")
}

func (f *YanCoreEntitlementFunding) Refund() error {
	if f.consumed <= 0 {
		return nil
	}
	return f.applyChange(f.consumed, model.YanCoreEntitlementLedgerRefund, "refund", "YanCore campaign entitlement refund")
}

func (f *YanCoreEntitlementFunding) reserve(delta int) error {
	if delta <= 0 {
		return nil
	}
	f.sequence++
	if err := f.applyChange(-delta, model.YanCoreEntitlementLedgerReserve, fmt.Sprintf("reserve:%d", f.sequence), "additional YanCore campaign entitlement reserve"); err != nil {
		return err
	}
	f.consumed += delta
	return nil
}

func (f *YanCoreEntitlementFunding) rollbackReserve(delta int) error {
	if delta <= 0 {
		return nil
	}
	if err := f.applyChange(delta, model.YanCoreEntitlementLedgerRefund, fmt.Sprintf("reserve-rollback:%d", f.sequence), "additional YanCore campaign reserve rollback"); err != nil {
		return err
	}
	f.consumed -= delta
	return nil
}

func (f *YanCoreEntitlementFunding) applyChange(amount int, entryType, phase, reason string) error {
	_, err := model.ApplyYanCoreEntitlementChange(model.YanCoreEntitlementChange{
		EntitlementId:  f.entitlementId,
		UserId:         f.userId,
		TokenId:        f.tokenId,
		RequestId:      f.requestId,
		IdempotencyKey: fmt.Sprintf("request:%s:campaign:%d:%s", f.requestId, f.entitlementId, phase),
		EntryType:      entryType,
		Amount:         amount,
		Reason:         reason,
	})
	return err
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := w.applyChange(-amount, model.QuotaLedgerTypeReserve, "preconsume", "model request reserve"); err != nil {
		return err
	}
	w.consumed = amount
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return w.applyChange(-delta, model.QuotaLedgerTypeSettlement, "settlement", "model request settlement")
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return w.applyChange(w.consumed, model.QuotaLedgerTypeRefund, "refund", "failed model request refund")
}

func (w *WalletFunding) reserve(delta int) error {
	if delta <= 0 {
		return nil
	}
	w.sequence++
	phase := fmt.Sprintf("reserve:%d", w.sequence)
	if err := w.applyChange(-delta, model.QuotaLedgerTypeReserve, phase, "additional model request reserve"); err != nil {
		return err
	}
	w.consumed += delta
	return nil
}

func (w *WalletFunding) rollbackReserve(delta int) error {
	if delta <= 0 {
		return nil
	}
	phase := fmt.Sprintf("reserve-rollback:%d", w.sequence)
	if err := w.applyChange(delta, model.QuotaLedgerTypeRefund, phase, "additional reserve rollback"); err != nil {
		return err
	}
	w.consumed -= delta
	return nil
}

func (w *WalletFunding) applyChange(amount int, entryType string, phase string, reason string) error {
	if !model.QuotaLedgerEnabled() && !yanCoreCampaignFundingEnabled() {
		if amount > 0 {
			return model.IncreaseUserQuota(w.userId, amount, false)
		}
		return model.DecreaseUserQuota(w.userId, -amount, false)
	}
	_, err := model.ApplyQuotaLedgerChange(model.QuotaLedgerChange{
		UserId:         w.userId,
		TokenId:        w.tokenId,
		RequestId:      w.requestId,
		IdempotencyKey: fmt.Sprintf("request:%s:wallet:%s", w.requestId, phase),
		EntryType:      entryType,
		FundingSource:  model.QuotaFundingPublicBenefit,
		Amount:         amount,
		Reason:         reason,
	})
	return err
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
