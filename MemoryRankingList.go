package game_rank_test

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryRankingList 内存排行榜

type Player struct {
	ID         string
	Score      int64
	UpdateTime time.Time // 记录分数最后更新时间，用于同分排序
}

// RankingSystem 排行榜系统
type RankingSystem struct {
	players map[string]*Player
	ranks   []*Player
	mu      sync.RWMutex
}

// NewRankingSystem 创建一个新的排行榜系统
func NewRankingSystem() *RankingSystem {
	return &RankingSystem{
		players: make(map[string]*Player),
		ranks:   make([]*Player, 0),
	}
}

// UpdateScore 更新玩家积分
// 如果玩家不存在则创建，存在则更新分数和时间戳
func (r *RankingSystem) UpdateScore(playerID string, score int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if player, exists := r.players[playerID]; exists {
		// 只有当分数变化时才更新时间戳，保证先达到高分的玩家排在前面
		if player.Score != score {
			player.Score = score
			player.UpdateTime = time.Now()
		}
	} else {
		// 新玩家
		r.players[playerID] = &Player{
			ID:         playerID,
			Score:      score,
			UpdateTime: time.Now(),
		}
	}
	r.ranks = r.getSortedPlayers()
}

// GetRank 查询玩家当前排名
func (r *RankingSystem) GetRank(playerID string) (int, *Player, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 检查玩家是否存在
	_, exists := r.players[playerID]
	if !exists {
		return 0, nil, fmt.Errorf("player %s not found", playerID)
	}

	// 查找玩家排名
	rank := 1
	for i, p := range r.ranks {
		if p.ID == playerID {
			// 考虑并列排名的情况
			if i > 0 && r.ranks[i-1].Score == p.Score {
				rank = i // 与前一名并列
			} else {
				rank = i + 1 // 新的排名
			}
			return rank, p, nil
		}
	}

	return 0, nil, fmt.Errorf("player %s not found in ranking", playerID)
}

// GetTopN 获取前N名玩家的分数和名次
func (r *RankingSystem) GetTopN(n int) ([]struct {
	Rank   int
	Player *Player
}, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be greater than 0")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	length := len(r.ranks)
	result := make([]struct {
		Rank   int
		Player *Player
	}, 0, min(n, length))

	// 计算排名，处理并列情况
	for i := 0; i < length && i < n; i++ {
		rank := i + 1
		// 如果当前玩家与前一位分数相同，则排名相同
		if i > 0 && r.ranks[i].Score == r.ranks[i-1].Score {
			rank = result[i-1].Rank
		}

		result = append(result, struct {
			Rank   int
			Player *Player
		}{rank, r.ranks[i]})
	}

	return result, nil
}

// GetPlayerRankRange 查询自己名次前后共N名玩家（包括自己）
func (r *RankingSystem) GetPlayerRankRange(playerID string, n int) ([]struct {
	Rank   int
	Player *Player
}, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be greater than 0")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 检查玩家是否存在
	if _, exists := r.players[playerID]; !exists {
		return nil, fmt.Errorf("player %s not found", playerID)
	}

	sortedPlayers := r.getSortedPlayers()

	// 找到玩家位置
	index := -1
	for i, p := range sortedPlayers {
		if p.ID == playerID {
			index = i
			break
		}
	}

	if index == -1 {
		return nil, fmt.Errorf("player %s not found in ranking", playerID)
	}

	// 计算需要获取的范围
	half := n / 2
	start := max(0, index-half)
	end := min(len(sortedPlayers), start+n)

	// 调整start，确保能取到足够的玩家
	if end-start < n {
		start = max(0, end-n)
	}

	result := make([]struct {
		Rank   int
		Player *Player
	}, 0, end-start)

	// 填充结果并计算排名
	for i := start; i < end; i++ {
		rank := i + 1
		if i > 0 && sortedPlayers[i].Score == sortedPlayers[i-1].Score {
			// 找到前一个不同分的玩家，计算正确排名
			for j := i - 1; j >= 0; j-- {
				if sortedPlayers[j].Score != sortedPlayers[i].Score {
					rank = j + 2
					break
				}
				if j == 0 {
					rank = 1
				}
			}
		}

		result = append(result, struct {
			Rank   int
			Player *Player
		}{rank, sortedPlayers[i]})
	}

	return result, nil
}

// getSortedPlayers 返回按排名规则排序的玩家列表
func (r *RankingSystem) getSortedPlayers() []*Player {
	// 将map转换为切片
	players := make([]*Player, 0, len(r.players))
	for _, p := range r.players {
		players = append(players, p)
	}

	// 排序：先按分数降序，分数相同则按更新时间升序（先达到该分数的排前面）
	sort.Slice(players, func(i, j int) bool {
		if players[i].Score != players[j].Score {
			return players[i].Score > players[j].Score
		}
		return players[i].UpdateTime.Before(players[j].UpdateTime)
	})

	return players
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
