package com.lsyl.tunnel.mobile.runtime

enum class IssueDisposition {
    TRANSIENT,
    FATAL,
    RULE_DISABLED,
    CONNECTION_ONLY
}

data class RuntimeIssue(
    val code: String,
    val message: String,
    val disposition: IssueDisposition,
    val ruleName: String? = null,
    val localPort: Int? = null
)
