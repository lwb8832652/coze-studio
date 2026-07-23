// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

type UserRepository struct { db *gorm.DB }
func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db:db} }

type UserPlanView struct { ID int64 `json:"id,string"`; Name, Description, Cycle, Currency string; PriceMicros, CreditGrantMicros int64; FeaturesJSON string }
type UserPackageView struct { ID int64 `json:"id,string"`; Name, Description, Currency string; PriceMicros, CreditMicros int64; ValidityDays int }
type UserSubscriptionView struct { ID int64 `json:"id,string"`; PlanID int64 `json:"plan_id,string"`; PlanName, Status string; CurrentPeriodStart, CurrentPeriodEnd time.Time; AutoRenew bool }
type UserLedgerView struct { ID int64 `json:"id,string"`; Direction, EntryType string; AmountMicros, AvailableAfterMicros int64; BusinessNo string; CreatedAt time.Time }
type UserUsageView struct { ID int64 `json:"id,string"`; TaskID int64 `json:"task_id,string"`; RunID, Provider, ModelID string; InputTokens, OutputTokens, CacheWriteTokens, CacheHitTokens, ChargeMicros int64; CreatedAt time.Time }
type UserOrderView struct { ID int64 `json:"id,string"`; OrderNo, OrderType, Status, PaymentStatus, FulfillmentStatus, Currency string; TotalMicros int64; CreatedAt time.Time }

func (r *UserRepository) GetSubscription(ctx context.Context, accountID int64) (*UserSubscriptionView,error) { var row struct{ID,PlanID int64;PlanName,Status string;CurrentPeriodStart,CurrentPeriodEnd time.Time;AutoRenew bool};err:=r.db.WithContext(ctx).Table("user_subscriptions AS s").Select("s.id,s.plan_id,p.name AS plan_name,s.status,s.current_period_start,s.current_period_end,s.auto_renew").Joins("JOIN subscription_plans p ON p.id = s.plan_id").Where("s.account_id = ? AND s.status IN ?",accountID,[]string{string(domainbilling.SubscriptionStatusActive),string(domainbilling.SubscriptionStatusPastDue)}).Order("s.current_period_end DESC,s.id DESC").First(&row).Error;if errors.Is(err,gorm.ErrRecordNotFound){return nil,domainbilling.ErrNotFound};if err!=nil{return nil,err};return &UserSubscriptionView{ID:row.ID,PlanID:row.PlanID,PlanName:row.PlanName,Status:row.Status,CurrentPeriodStart:row.CurrentPeriodStart,CurrentPeriodEnd:row.CurrentPeriodEnd,AutoRenew:row.AutoRenew},nil }

func (r *UserRepository) ListPlans(ctx context.Context)([]UserPlanView,error){var rows []struct{ID int64;Name,Description,Cycle,Currency,FeaturesJSON string;PriceMicros,CreditGrantMicros int64};err:=r.db.WithContext(ctx).Table("subscription_plans AS p").Select("p.id,p.name,p.description,v.billing_cycle AS cycle,v.currency,v.features_json,v.price_micros,v.credit_grant_micros").Joins("JOIN subscription_plan_versions v ON v.plan_id=p.id AND v.version=p.current_version").Where("p.status = ? AND p.deleted_at IS NULL",string(domainbilling.CatalogStatusPublished)).Order("p.sort_order ASC,p.id ASC").Scan(&rows).Error;if err!=nil{return nil,err};out:=make([]UserPlanView,0,len(rows));for _,v:=range rows{out=append(out,UserPlanView{ID:v.ID,Name:v.Name,Description:v.Description,Cycle:v.Cycle,Currency:v.Currency,FeaturesJSON:v.FeaturesJSON,PriceMicros:v.PriceMicros,CreditGrantMicros:v.CreditGrantMicros})};return out,nil}
func (r *UserRepository) ListPackages(ctx context.Context)([]UserPackageView,error){var rows []creditPackagePO;if err:=r.db.WithContext(ctx).Where("status = ? AND deleted_at IS NULL",string(domainbilling.CatalogStatusPublished)).Order("sort_order ASC,id ASC").Find(&rows).Error;err!=nil{return nil,err};out:=make([]UserPackageView,0,len(rows));for _,v:=range rows{out=append(out,UserPackageView{ID:v.ID,Name:v.Name,Description:v.Description,Currency:v.Currency,PriceMicros:v.PriceMicros,CreditMicros:v.CreditMicros,ValidityDays:v.ValidityDays})};return out,nil}
func (r *UserRepository) ListLedger(ctx context.Context,accountID int64)([]UserLedgerView,error){var rows []ledgerPO;if err:=r.db.WithContext(ctx).Where("account_id = ?",accountID).Order("created_at DESC,id DESC").Limit(500).Find(&rows).Error;err!=nil{return nil,err};out:=make([]UserLedgerView,0,len(rows));for _,v:=range rows{out=append(out,UserLedgerView{ID:v.ID,Direction:v.Direction,EntryType:v.EntryType,AmountMicros:v.AmountMicros,AvailableAfterMicros:v.AvailableAfterMicros,BusinessNo:v.BusinessNo,CreatedAt:v.CreatedAt})};return out,nil}
func (r *UserRepository) ListUsage(ctx context.Context,accountID int64)([]UserUsageView,error){var rows []usageRecordPO;if err:=r.db.WithContext(ctx).Where("account_id = ?",accountID).Order("created_at DESC,id DESC").Limit(500).Find(&rows).Error;err!=nil{return nil,err};out:=make([]UserUsageView,0,len(rows));for _,v:=range rows{out=append(out,UserUsageView{ID:v.ID,TaskID:v.TaskID,RunID:v.RunID,Provider:v.Provider,ModelID:v.ModelID,InputTokens:v.InputTokens,OutputTokens:v.OutputTokens,CacheWriteTokens:v.CacheWriteTokens,CacheHitTokens:v.CacheHitTokens,ChargeMicros:v.ChargeMicros,CreatedAt:v.CreatedAt})};return out,nil}
func (r *UserRepository) ListOrders(ctx context.Context,userID int64)([]UserOrderView,error){var rows []orderPO;if err:=r.db.WithContext(ctx).Where("user_id = ?",userID).Order("created_at DESC,id DESC").Limit(500).Find(&rows).Error;err!=nil{return nil,err};out:=make([]UserOrderView,0,len(rows));for _,v:=range rows{out=append(out,UserOrderView{ID:v.ID,OrderNo:v.OrderNo,OrderType:v.OrderType,Status:v.Status,PaymentStatus:v.PaymentStatus,FulfillmentStatus:v.FulfillmentStatus,Currency:v.Currency,TotalMicros:v.TotalMicros,CreatedAt:v.CreatedAt})};return out,nil}
