package file

import (
	"sort"
	"sync"
)

// OrderedSyncMap 是一个有序的并发安全map
// 内部使用map存储数据，slice维护key的顺序
type OrderedSyncMap struct {
	mu     sync.RWMutex
	data   map[int]interface{}
	keys   []int        // 保持有序的key列表
	keySet map[int]bool // 快速判断key是否存在
}

// NewOrderedSyncMap 创建新的有序同步map
func NewOrderedSyncMap() *OrderedSyncMap {
	return &OrderedSyncMap{
		data:   make(map[int]interface{}),
		keys:   make([]int, 0),
		keySet: make(map[int]bool),
	}
}

// Store 存储键值对，保持key有序
func (m *OrderedSyncMap) Store(key interface{}, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := key.(int)

	// 如果key不存在，需要插入到有序位置
	if !m.keySet[k] {
		// 二分查找插入位置
		pos := sort.SearchInts(m.keys, k)
		// 插入到正确位置
		m.keys = append(m.keys, 0)
		copy(m.keys[pos+1:], m.keys[pos:])
		m.keys[pos] = k
		m.keySet[k] = true
	}

	m.data[k] = value
}

// Load 加载指定key的值
func (m *OrderedSyncMap) Load(key interface{}) (value interface{}, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	k := key.(int)
	value, ok = m.data[k]
	return
}

// Delete 删除指定key
func (m *OrderedSyncMap) Delete(key interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := key.(int)

	if m.keySet[k] {
		// 从有序列表中删除
		pos := sort.SearchInts(m.keys, k)
		if pos < len(m.keys) && m.keys[pos] == k {
			m.keys = append(m.keys[:pos], m.keys[pos+1:]...)
		}
		delete(m.keySet, k)
		delete(m.data, k)
	}
}

// Range 按key顺序遍历所有元素
// 如果f返回false，停止遍历
func (m *OrderedSyncMap) Range(f func(key, value interface{}) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, k := range m.keys {
		if v, ok := m.data[k]; ok {
			if !f(k, v) {
				break
			}
		}
	}
}

// LoadOrStore 加载或存储
func (m *OrderedSyncMap) LoadOrStore(key interface{}, value interface{}) (actual interface{}, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := key.(int)

	if v, ok := m.data[k]; ok {
		return v, true
	}

	// 不存在，需要存储
	// 二分查找插入位置
	pos := sort.SearchInts(m.keys, k)
	// 插入到正确位置
	m.keys = append(m.keys, 0)
	copy(m.keys[pos+1:], m.keys[pos:])
	m.keys[pos] = k
	m.keySet[k] = true
	m.data[k] = value

	return value, false
}

// LoadAndDelete 加载并删除
func (m *OrderedSyncMap) LoadAndDelete(key interface{}) (value interface{}, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := key.(int)

	if v, ok := m.data[k]; ok {
		// 从有序列表中删除
		pos := sort.SearchInts(m.keys, k)
		if pos < len(m.keys) && m.keys[pos] == k {
			m.keys = append(m.keys[:pos], m.keys[pos+1:]...)
		}
		delete(m.keySet, k)
		delete(m.data, k)
		return v, true
	}

	return nil, false
}

// Len 返回元素个数
func (m *OrderedSyncMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Keys 返回所有key的有序列表
func (m *OrderedSyncMap) Keys() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]int, len(m.keys))
	copy(result, m.keys)
	return result
}
