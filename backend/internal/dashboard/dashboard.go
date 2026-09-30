package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateofu/movank-fullstack/backend/internal/product"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type Snapshot struct {
	Date       string `json:"date"`
	Currency   string `json:"currency"`
	PaidSales  int64  `json:"paid_sales"`
	TotalMinor int64  `json:"total_minor"`
}

type Service struct {
	pool   *pgxpool.Pool
	cache  *redis.Client
	flight singleflight.Group
}

func New(pool *pgxpool.Pool, addr string) *Service {
	var client *redis.Client
	if addr != "" {
		client = redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 200 * time.Millisecond, ReadTimeout: 200 * time.Millisecond, WriteTimeout: 200 * time.Millisecond, MaxRetries: -1, ContextTimeoutEnabled: true})
	}
	return &Service{pool: pool, cache: client}
}

func (s *Service) Close() {
	if s.cache != nil {
		_ = s.cache.Close()
	}
}

var saveSnapshot = redis.NewScript(`
local old=redis.call('GET',KEYS[1])
if old then
 local ok,value=pcall(cjson.decode,old)
 if ok and tonumber(value.paid_sales) and tonumber(value.paid_sales)>tonumber(ARGV[1]) then return old end
end
redis.call('SET',KEYS[1],ARGV[2],'EX',30)
return ARGV[2]
`)

func (s *Service) Today(ctx context.Context, merchantID string, refresh bool) (Snapshot, error) {
	if !product.ValidID(merchantID) {
		return Snapshot{}, errors.New("invalid merchant")
	}
	day := time.Now().UTC().Truncate(24 * time.Hour)
	key := "dashboard:" + merchantID + ":" + day.Format("2006-01-02")
	if !refresh && s.cache != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		data, err := s.cache.Get(cacheCtx, key).Bytes()
		cancel()
		var result Snapshot
		if err == nil && json.Unmarshal(data, &result) == nil && result.Date == day.Format("2006-01-02") && result.Currency == "COP" && result.PaidSales >= 0 && result.TotalMinor >= 0 {
			return result, nil
		}
	}
	load := func() (any, error) {
		queryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result := Snapshot{Date: day.Format("2006-01-02"), Currency: "COP"}
		err := s.pool.QueryRow(queryCtx, `SELECT count(*),COALESCE(sum(amount_minor),0)::bigint FROM public.payments
   WHERE merchant_id=$1 AND status='APPROVED' AND completed_at >= $2 AND completed_at < $3`, merchantID, day, day.Add(24*time.Hour)).Scan(&result.PaidSales, &result.TotalMinor)
		if err != nil {
			return Snapshot{}, err
		}
		if s.cache != nil {
			data, _ := json.Marshal(result)
			cacheCtx, stop := context.WithTimeout(queryCtx, 250*time.Millisecond)
			saved, cacheErr := saveSnapshot.Run(cacheCtx, s.cache, []string{key}, result.PaidSales, string(data)).Text()
			stop()
			var latest Snapshot
			if cacheErr == nil && json.Unmarshal([]byte(saved), &latest) == nil && latest.Date == result.Date && latest.Currency == "COP" && latest.PaidSales >= result.PaidSales {
				result = latest
			}
		}
		return result, nil
	}
	if refresh {
		value, err := load()
		if err != nil {
			return Snapshot{}, err
		}
		return value.(Snapshot), nil
	}
	ch := s.flight.DoChan(key, load)
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return Snapshot{}, result.Err
		}
		return result.Val.(Snapshot), nil
	}
}
