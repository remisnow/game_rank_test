package game_rank_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisRankingList 基于Redis ZSet的排行榜系统
type RedisRankingList struct {
	client *redis.Client
	key    string          // Redis中存储排行榜的键名
	ctx    context.Context // 上下文
}

// PlayerRank 玩家排名信息
type PlayerRank struct {
	PlayerID string
	Score    int64
	Rank     int
}

// NewRedisRankingSystem 创建一个新的Redis排行榜系统
func NewRedisRankingSystem(addr string, password string, db int, key string) *RedisRankingList {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("无法连接到Redis: %v", err))
	}

	return &RedisRankingList{
		client: client,
		key:    key,
		ctx:    ctx,
	}
}

// UpdateScore 更新玩家积分

func (r *RedisRankingList) UpdateScore(playerID string, score int64) error {
	// 生成复合分数：主分数左移40位，减去当前时间戳（毫秒）方便时间戳存储
	timestamp := time.Now().UnixMilli()
	compositeScore := float64(score<<40 - timestamp)

	return r.client.ZAdd(r.ctx, r.key, &redis.Z{
		Score:  compositeScore,
		Member: playerID,
	}).Err()
}

// GetRealScore 从复合分数中提取真实分数
func (r *RedisRankingList) GetRealScore(compositeScore float64) int64 {
	return int64(compositeScore) >> 40
}

// GetRank 查询玩家当前排名
func (r *RedisRankingList) GetRank(playerID string) (int, int64, error) {
	// ZRank返回的是升序排名，我们需要转换为降序排名
	rank, err := r.client.ZRank(r.ctx, r.key, playerID).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("获取排名失败: %v", err)
	}

	// 获取玩家分数
	score, err := r.client.ZScore(r.ctx, r.key, playerID).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("获取分数失败: %v", err)
	}

	// 转换为降序排名（+1是因为排名从1开始）
	total, err := r.client.ZCard(r.ctx, r.key).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("获取总人数失败: %v", err)
	}

	// 计算实际排名（降序）
	actualRank := int(total - rank)
	realScore := r.GetRealScore(score)

	return actualRank, realScore, nil
}

// GetTopN 获取前N名玩家的分数和名次
func (r *RedisRankingList) GetTopN(n int) ([]PlayerRank, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n必须大于0")
	}

	// ZRange返回升序排列，我们取前n个就是分数最高的n个
	results, err := r.client.ZRangeWithScores(r.ctx, r.key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("获取前N名失败: %v", err)
	}

	rankings := make([]PlayerRank, 0, len(results))
	for i, z := range results {
		playerID, ok := z.Member.(string)
		if !ok {
			continue
		}

		// 处理并列排名
		rank := i + 1
		if i > 0 && r.GetRealScore(results[i].Score) == r.GetRealScore(results[i-1].Score) {
			rank = rankings[i-1].Rank
		}

		rankings = append(rankings, PlayerRank{
			PlayerID: playerID,
			Score:    r.GetRealScore(z.Score),
			Rank:     rank,
		})
	}

	return rankings, nil
}

// GetPlayerRankRange 查询自己名次前后共N名玩家（包括自己）
func (r *RedisRankingList) GetPlayerRankRange(playerID string, n int) ([]PlayerRank, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n必须大于0")
	}

	// 获取玩家当前排名（升序）
	rank, err := r.client.ZRank(r.ctx, r.key, playerID).Result()
	if err != nil {
		return nil, fmt.Errorf("获取玩家排名失败: %v", err)
	}

	// 计算需要查询的范围
	half := n / 2
	start := rank - int64(half)
	if start < 0 {
		start = 0
	}
	end := start + int64(n) - 1

	// 获取范围内的玩家
	results, err := r.client.ZRangeWithScores(r.ctx, r.key, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("获取周围玩家失败: %v", err)
	}

	// 转换为PlayerRank列表
	rankings := make([]PlayerRank, 0, len(results))
	for _, z := range results {
		playerID, ok := z.Member.(string)
		if !ok {
			continue
		}

		// 获取该玩家的实际排名（降序）
		playerAscRank, err := r.client.ZRank(r.ctx, r.key, playerID).Result()
		if err != nil {
			continue
		}

		total, err := r.client.ZCard(r.ctx, r.key).Result()
		if err != nil {
			return nil, fmt.Errorf("获取总人数失败: %v", err)
		}

		actualRank := int(total - playerAscRank)

		rankings = append(rankings, PlayerRank{
			PlayerID: playerID,
			Score:    r.GetRealScore(z.Score),
			Rank:     actualRank,
		})
	}

	// 按排名排序（确保顺序正确）
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].Rank < rankings[j].Rank
	})

	return rankings, nil
}

// GetTotalPlayers 获取总玩家数
func (r *RedisRankingList) GetTotalPlayers() (int64, error) {
	return r.client.ZCard(r.ctx, r.key).Result()
}

// RemovePlayer 移除玩家
func (r *RedisRankingList) RemovePlayer(playerID string) error {
	return r.client.ZRem(r.ctx, r.key, playerID).Err()
}
