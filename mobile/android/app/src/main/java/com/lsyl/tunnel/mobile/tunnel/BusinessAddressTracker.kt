package com.lsyl.tunnel.mobile.tunnel

class BusinessAddressTracker(private val onChanged: (String?) -> Unit = {}) {
    private val active = LinkedHashMap<Long, String>()
    private var nextId = 0L
    private var closed = false

    fun begin(address: String): () -> Unit {
        if (address.isBlank()) return {}
        val id = synchronized(this) {
            if (closed) return {}
            val previous = active.values.lastOrNull()
            val next = ++nextId
            active[next] = address
            if (previous != address) onChanged(address)
            next
        }
        return {
            synchronized(this) {
                val previous = active.values.lastOrNull()
                active.remove(id)
                val current = active.values.lastOrNull()
                if (previous != current) onChanged(current)
            }
        }
    }

    @Synchronized fun current(): String? = active.values.lastOrNull()

    @Synchronized fun clear() {
        closed = true
        if (active.isNotEmpty()) {
            active.clear()
            onChanged(null)
        }
    }
}
