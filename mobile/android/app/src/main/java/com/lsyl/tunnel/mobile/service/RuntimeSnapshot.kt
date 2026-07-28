package com.lsyl.tunnel.mobile.service

import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import org.json.JSONArray
import org.json.JSONObject

enum class TunnelPhase {
    DISCONNECTED,
    STARTING,
    CONNECTED,
    DEGRADED,
    WAITING_NETWORK,
    STOPPING,
    FAILED
}

data class RuntimeSnapshot(
    val phase: TunnelPhase,
    val summary: String,
    val listenerCount: Int = 0,
    val issues: List<RuntimeIssue> = emptyList()
) {
    fun toJson(): String = JSONObject().apply {
        put("phase", phase.name)
        put("summary", summary)
        put("listener_count", listenerCount)
        put("issues", JSONArray().also { array ->
            issues.forEach { issue ->
                array.put(JSONObject().apply {
                    put("code", issue.code)
                    put("message", issue.message)
                    put("disposition", issue.disposition.name)
                    issue.ruleName?.let { put("rule_name", it) }
                    issue.localPort?.let { put("local_port", it) }
                })
            }
        })
    }.toString()

    companion object {
        fun fromJson(text: String): RuntimeSnapshot {
            val json = JSONObject(text)
            val issueArray = json.optJSONArray("issues") ?: JSONArray()
            val issues = buildList {
                for (index in 0 until issueArray.length()) {
                    val issue = issueArray.getJSONObject(index)
                    add(
                        RuntimeIssue(
                            code = issue.getString("code"),
                            message = issue.getString("message"),
                            disposition = IssueDisposition.valueOf(issue.getString("disposition")),
                            ruleName = issue.optStringOrNull("rule_name"),
                            localPort = if (issue.has("local_port")) issue.getInt("local_port") else null
                        )
                    )
                }
            }
            return RuntimeSnapshot(
                phase = TunnelPhase.valueOf(json.getString("phase")),
                summary = json.getString("summary"),
                listenerCount = json.optInt("listener_count", 0),
                issues = issues
            )
        }

        private fun JSONObject.optStringOrNull(name: String): String? =
            if (has(name) && !isNull(name)) optString(name).takeIf { it.isNotBlank() } else null
    }
}
