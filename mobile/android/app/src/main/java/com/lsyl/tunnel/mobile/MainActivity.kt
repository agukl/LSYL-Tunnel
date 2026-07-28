package com.lsyl.tunnel.mobile

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import com.lsyl.tunnel.mobile.profile.ForwardTextConfig
import com.lsyl.tunnel.mobile.profile.ImportedProfile
import com.lsyl.tunnel.mobile.profile.ProfileImporter
import com.lsyl.tunnel.mobile.profile.ProfileStore
import com.lsyl.tunnel.mobile.service.RuntimePresenter
import com.lsyl.tunnel.mobile.service.RuntimeSnapshot
import com.lsyl.tunnel.mobile.service.RuntimeStateStore
import com.lsyl.tunnel.mobile.service.TunnelPhase
import com.lsyl.tunnel.mobile.service.TunnelForegroundService
import java.time.format.DateTimeFormatter

class MainActivity : Activity() {
    private lateinit var userText: TextView
    private lateinit var expiryText: TextView
    private lateinit var statusText: TextView
    private lateinit var detailsText: TextView
    private lateinit var importBtn: Button
    private lateinit var connectBtn: Button
    private lateinit var stopBtn: Button
    private lateinit var refreshBtn: Button
    private lateinit var editBtn: Button
    private lateinit var deleteBtn: Button
    private lateinit var store: ProfileStore
    private lateinit var runtimeStore: RuntimeStateStore
    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            val raw = intent?.getStringExtra(TunnelForegroundService.EXTRA_SNAPSHOT) ?: return
            val snapshot = try {
                RuntimeSnapshot.fromJson(raw)
            } catch (_: Exception) {
                return
            }
            updateRuntime(snapshot)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = ProfileStore(this)
        runtimeStore = RuntimeStateStore(this)
        configureSystemBars()
        buildUi()
        handleViewIntent(intent)
        refreshProfileView()
        requestNotificationPermissionIfNeeded()
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        if (intent != null) handleViewIntent(intent)
    }

    override fun onResume() {
        super.onResume()
        val filter = IntentFilter(TunnelForegroundService.ACTION_STATUS)
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(
                statusReceiver,
                filter,
                TunnelForegroundService.INTERNAL_STATUS_PERMISSION,
                null,
                Context.RECEIVER_NOT_EXPORTED
            )
        } else {
            registerReceiver(statusReceiver, filter, TunnelForegroundService.INTERNAL_STATUS_PERMISSION, null)
        }
        refreshProfileView()
    }

    override fun onPause() {
        unregisterReceiver(statusReceiver)
        super.onPause()
    }

    @Deprecated("Deprecated in Android API, kept to avoid AndroidX dependency.")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQ_IMPORT && resultCode == RESULT_OK) {
            data?.data?.let { importProfile(it) }
        }
    }

    private fun buildUi() {
        val scroll = ScrollView(this).apply {
            isFillViewport = true
            background = GradientDrawable(
                GradientDrawable.Orientation.TOP_BOTTOM,
                intArrayOf(PAGE_TOP, PAGE_BOTTOM)
            )
        }
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(24), dp(36), dp(24), dp(22))
        }
        val title = TextView(this).apply {
            text = "LSYL Tunnel"
            textSize = 30f
            typeface = Typeface.DEFAULT_BOLD
            setTextColor(Color.rgb(7, 62, 59))
        }

        val profileCard = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(22), dp(18), dp(22), dp(18))
            background = rounded(Color.WHITE, 24)
            elevation = dp(1).toFloat()
        }
        val profileTitle = TextView(this).apply {
            text = "当前配置"
            textSize = 17f
            typeface = Typeface.DEFAULT_BOLD
            setTextColor(Color.rgb(20, 68, 64))
        }
        statusText = TextView(this).apply {
            textSize = 15f
            typeface = Typeface.DEFAULT_BOLD
            setPadding(0, dp(8), 0, 0)
        }
        userText = TextView(this).apply { textSize = 18f }
        expiryText = TextView(this).apply { textSize = 15f }
        listOf(statusText, userText, expiryText).forEach {
            it.setTextColor(Color.rgb(56, 82, 80))
            it.setPadding(0, dp(8), 0, 0)
        }
        detailsText = TextView(this).apply {
            textSize = 14f
            setTextColor(Color.rgb(150, 79, 0))
            setPadding(0, dp(10), 0, 0)
            visibility = View.GONE
        }
        profileCard.addView(profileTitle)
        profileCard.addView(statusText)
        profileCard.addView(userText)
        profileCard.addView(expiryText)
        profileCard.addView(detailsText)

        val actionTitle = TextView(this).apply {
            text = "操作"
            textSize = 17f
            typeface = Typeface.DEFAULT_BOLD
            setTextColor(Color.rgb(20, 68, 64))
        }
        connectBtn = actionButton("连接", ButtonStyle.PRIMARY).apply {
            setOnClickListener { startTunnel() }
        }
        stopBtn = actionButton("断开连接", ButtonStyle.WARNING).apply {
            setOnClickListener { stopTunnel() }
        }
        refreshBtn = actionButton("重新检查", ButtonStyle.GHOST).apply {
            setOnClickListener { refreshTunnel() }
        }

        editBtn = smallActionButton("编辑转发", ButtonStyle.GHOST).apply {
            setOnClickListener { editForwards() }
        }

        val configRow = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER
        }
        importBtn = smallActionButton("导入配置", ButtonStyle.GHOST).apply {
            setOnClickListener { openProfilePicker() }
        }
        deleteBtn = smallActionButton("删除配置", ButtonStyle.GHOST_DANGER).apply {
            setOnClickListener { deleteProfile() }
        }
        configRow.addView(importBtn, smallButtonParams(endMargin = dp(10)))
        configRow.addView(deleteBtn, smallButtonParams(startMargin = dp(10)))

        root.addView(title)
        addWithTop(root, profileCard, dp(24))
        addWithTop(root, actionTitle, dp(22))
        addWithTop(root, connectBtn, dp(12))
        addWithTop(root, stopBtn, dp(12))
        addWithTop(root, refreshBtn, dp(12))
        root.addView(View(this), LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
        addWithTop(root, editBtn, dp(24))
        addWithTop(root, configRow, dp(10))

        scroll.addView(root, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        setContentView(scroll)
    }

    private fun refreshProfileView() {
        val loaded = store.load()
        if (loaded == null) {
            userText.text = "用户：未导入"
            expiryText.text = "有效期：-"
            updateRuntime(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接"), hasProfile = false)
            return
        }
        userText.text = "用户：${loaded.profile.username}"
        expiryText.text = "有效期：${formatExpiry(loaded.profile.savedCredential.expiresAt)}"
        updateRuntime(runtimeStore.loadSnapshot(), hasProfile = true)
    }

    private fun openProfilePicker() {
        val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            type = "*/*"
        }
        startActivityForResult(intent, REQ_IMPORT)
    }

    private fun handleViewIntent(intent: Intent) {
        if (intent.action == Intent.ACTION_VIEW) {
            intent.data?.let { importProfile(it) }
        }
    }

    private fun importProfile(uri: Uri) {
        if (!canReplaceProfileNow()) {
            showError("请先断开连接，再导入配置")
            return
        }
        try {
            val imported = ProfileImporter.importFromUri(this, uri)
            showImportConfirm(imported)
        } catch (err: Exception) {
            showError(err.message ?: "导入失败")
        }
    }

    private fun showImportConfirm(imported: ImportedProfile) {
        val profile = imported.profile
        val message = "用户：${profile.username}\n有效期至：${formatExpiry(profile.savedCredential.expiresAt)}\n\n此配置由管理员生成，导入后可直接连接。"
        AlertDialog.Builder(this)
            .setTitle("导入连接配置")
            .setMessage(message)
            .setNegativeButton("取消", null)
            .setPositiveButton("导入") { _, _ ->
                if (!canReplaceProfileNow()) {
                    showError("请先断开连接，再导入配置")
                    return@setPositiveButton
                }
                store.save(imported)
                runtimeStore.setDesiredRunning(false)
                runtimeStore.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "已导入"))
                refreshProfileView()
                Toast.makeText(this, "配置已导入", Toast.LENGTH_SHORT).show()
            }
            .show()
    }

    private fun startTunnel() {
        if (store.load() == null) {
            showError("请先导入连接配置")
            return
        }
        runtimeStore.setDesiredRunning(true)
        try {
            val intent = TunnelForegroundService.startIntent(this)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) startForegroundService(intent) else startService(intent)
            updateRuntime(RuntimeSnapshot(TunnelPhase.STARTING, "读取配置"))
        } catch (err: Exception) {
            runtimeStore.setDesiredRunning(false)
            showError("无法启动隧道服务：${err.message ?: "系统拒绝启动"}")
            refreshProfileView()
        }
    }

    private fun stopTunnel() {
        val stopping = RuntimeSnapshot(TunnelPhase.STOPPING, "正在断开")
        runtimeStore.setDesiredRunning(false)
        runtimeStore.publish(stopping)
        startService(TunnelForegroundService.stopIntent(this))
        updateRuntime(stopping)
    }

    private fun refreshTunnel() {
        if (!runtimeStore.desiredRunning()) return
        startService(TunnelForegroundService.refreshIntent(this))
        updateRuntime(
            RuntimeSnapshot(
                phase = TunnelPhase.STARTING,
                summary = "重新检查连接",
                listenerCount = runtimeStore.loadSnapshot().listenerCount
            )
        )
    }

    private fun deleteProfile() {
        AlertDialog.Builder(this)
            .setTitle("删除配置")
            .setMessage("删除后需要重新导入管理员下发的配置。")
            .setNegativeButton("取消", null)
            .setPositiveButton("删除") { _, _ ->
                runtimeStore.setDesiredRunning(false)
                startService(TunnelForegroundService.stopIntent(this))
                store.delete()
                runtimeStore.publish(RuntimeSnapshot(TunnelPhase.DISCONNECTED, "未连接"))
                refreshProfileView()
                Toast.makeText(this, "配置已删除", Toast.LENGTH_SHORT).show()
            }
            .show()
    }

    private fun editForwards() {
        val loaded = store.load() ?: run {
            showError("请先导入连接配置")
            return
        }
        val presentation = RuntimePresenter.present(
            runtimeStore.loadSnapshot(),
            hasProfile = true,
            desiredRunning = runtimeStore.desiredRunning()
        )
        if (!presentation.canEditConfig) {
            showError("请先断开连接，再编辑转发配置")
            return
        }

        val errorText = TextView(this).apply {
            textSize = 13f
            setTextColor(Color.rgb(174, 55, 50))
            visibility = View.GONE
            setPadding(0, 0, 0, dp(8))
        }
        val editor = EditText(this).apply {
            setText(ForwardTextConfig.render(loaded.profile.forwards))
            setSelection(text.length)
            typeface = Typeface.MONOSPACE
            textSize = 14f
            gravity = Gravity.TOP or Gravity.START
            inputType = InputType.TYPE_CLASS_TEXT or
                InputType.TYPE_TEXT_FLAG_MULTI_LINE or
                InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
            minLines = 10
            maxLines = 18
            setHorizontallyScrolling(false)
            setPadding(dp(12), dp(10), dp(12), dp(10))
            background = rounded(Color.rgb(247, 251, 250), 8)
        }
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(8), dp(20), 0)
            addView(errorText, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT))
            addView(editor, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dp(320)))
        }
        val dialog = AlertDialog.Builder(this)
            .setTitle("编辑转发配置")
            .setView(content)
            .setNegativeButton("取消", null)
            .setPositiveButton("保存", null)
            .create()
        dialog.setOnShowListener {
            dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener {
                try {
                    val forwards = ForwardTextConfig.parse(editor.text.toString())
                    store.updateForwards(forwards)
                    errorText.visibility = View.GONE
                    dialog.dismiss()
                    refreshProfileView()
                    Toast.makeText(this, "转发配置已保存", Toast.LENGTH_SHORT).show()
                } catch (err: Exception) {
                    errorText.text = err.message ?: "转发配置无效"
                    errorText.visibility = View.VISIBLE
                }
            }
        }
        dialog.show()
    }

    private fun updateRuntime(snapshot: RuntimeSnapshot, hasProfile: Boolean = store.load() != null) {
        val presentation = RuntimePresenter.present(snapshot, hasProfile, runtimeStore.desiredRunning())
        statusText.text = "状态：${presentation.status}"
        statusText.setTextColor(statusColor(snapshot.phase))
        detailsText.text = presentation.details
        detailsText.visibility = if (presentation.details.isBlank()) View.GONE else View.VISIBLE
        if (!::connectBtn.isInitialized) return
        connectBtn.isEnabled = presentation.canConnect
        stopBtn.isEnabled = presentation.canDisconnect && snapshot.phase != TunnelPhase.STOPPING
        refreshBtn.isEnabled = presentation.canRefresh
        editBtn.isEnabled = presentation.canEditConfig
        importBtn.isEnabled = presentation.canReplaceProfile
        deleteBtn.isEnabled = hasProfile && presentation.canReplaceProfile
        applyButtonStyle(connectBtn, ButtonStyle.PRIMARY)
        applyButtonStyle(stopBtn, ButtonStyle.WARNING)
        applyButtonStyle(refreshBtn, ButtonStyle.GHOST)
        applyButtonStyle(editBtn, ButtonStyle.GHOST, small = true)
        applyButtonStyle(importBtn, ButtonStyle.GHOST, small = true)
        applyButtonStyle(deleteBtn, ButtonStyle.GHOST_DANGER, small = true)
    }

    private fun canReplaceProfileNow(): Boolean = RuntimePresenter.present(
        runtimeStore.loadSnapshot(),
        hasProfile = store.load() != null,
        desiredRunning = runtimeStore.desiredRunning()
    ).canReplaceProfile

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIFY)
        }
    }

    private fun formatExpiry(value: String): String = try {
        java.time.OffsetDateTime.parse(value).format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm"))
    } catch (_: Exception) {
        value
    }

    private fun showError(message: String) {
        AlertDialog.Builder(this).setTitle("提示").setMessage(message).setPositiveButton("确定", null).show()
    }

    private fun configureSystemBars() {
        window.statusBarColor = PAGE_TOP
        window.navigationBarColor = PAGE_BOTTOM
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            var flags = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                flags = flags or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
            }
            window.decorView.systemUiVisibility = flags
        }
    }

    private fun actionButton(text: String, style: ButtonStyle): Button = Button(this).apply {
        this.text = text
        textSize = 16f
        typeface = Typeface.DEFAULT_BOLD
        isAllCaps = false
        minHeight = dp(54)
        setPadding(dp(12), 0, dp(12), 0)
        applyButtonStyle(this, style)
    }

    private fun smallActionButton(text: String, style: ButtonStyle): Button = Button(this).apply {
        this.text = text
        textSize = 13f
        typeface = Typeface.DEFAULT_BOLD
        isAllCaps = false
        minHeight = dp(38)
        minWidth = dp(112)
        setPadding(dp(14), 0, dp(14), 0)
        applyButtonStyle(this, style, small = true)
    }

    private fun applyButtonStyle(button: Button, style: ButtonStyle, small: Boolean = false) {
        val enabled = button.isEnabled
        val bg = when {
            !enabled -> Color.rgb(222, 234, 232)
            style == ButtonStyle.PRIMARY -> Color.rgb(0, 137, 126)
            style == ButtonStyle.WARNING -> Color.rgb(255, 248, 232)
            style == ButtonStyle.GHOST_DANGER -> Color.rgb(255, 243, 242)
            else -> Color.rgb(235, 247, 245)
        }
        val fg = when {
            !enabled -> Color.rgb(130, 153, 150)
            style == ButtonStyle.PRIMARY -> Color.WHITE
            style == ButtonStyle.WARNING -> Color.rgb(136, 88, 0)
            style == ButtonStyle.GHOST_DANGER -> Color.rgb(174, 55, 50)
            else -> Color.rgb(13, 104, 97)
        }
        button.setTextColor(fg)
        button.background = rounded(bg, if (small) 14 else 20)
    }

    private fun addWithTop(parent: LinearLayout, view: View, top: Int) {
        parent.addView(view, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
            topMargin = top
        })
    }

    private fun smallButtonParams(startMargin: Int = 0, endMargin: Int = 0): LinearLayout.LayoutParams =
        LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, dp(40)).apply {
            leftMargin = startMargin
            rightMargin = endMargin
        }

    private fun rounded(color: Int, radiusDp: Int): GradientDrawable =
        GradientDrawable().apply {
            setColor(color)
            cornerRadius = dp(radiusDp).toFloat()
        }

    private fun statusColor(phase: TunnelPhase): Int = when (phase) {
        TunnelPhase.CONNECTED -> Color.rgb(0, 115, 96)
        TunnelPhase.DEGRADED,
        TunnelPhase.WAITING_NETWORK -> Color.rgb(172, 96, 0)
        TunnelPhase.FAILED -> Color.rgb(174, 55, 50)
        TunnelPhase.STARTING,
        TunnelPhase.STOPPING -> Color.rgb(26, 95, 139)
        TunnelPhase.DISCONNECTED -> Color.rgb(9, 82, 76)
    }

    private fun dp(value: Int): Int = (value * resources.displayMetrics.density + 0.5f).toInt()

    private enum class ButtonStyle {
        PRIMARY,
        WARNING,
        GHOST,
        GHOST_DANGER
    }

    companion object {
        private val PAGE_TOP = Color.rgb(232, 250, 247)
        private val PAGE_BOTTOM = Color.rgb(248, 252, 251)
        private const val REQ_IMPORT = 1001
        private const val REQ_NOTIFY = 1002
    }
}
