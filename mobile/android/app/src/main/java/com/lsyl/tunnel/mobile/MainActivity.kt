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
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import com.lsyl.tunnel.mobile.profile.ImportedProfile
import com.lsyl.tunnel.mobile.profile.ProfileImporter
import com.lsyl.tunnel.mobile.profile.ProfileStore
import com.lsyl.tunnel.mobile.service.TunnelForegroundService
import java.time.format.DateTimeFormatter

class MainActivity : Activity() {
    private lateinit var userText: TextView
    private lateinit var expiryText: TextView
    private lateinit var statusText: TextView
    private lateinit var importBtn: Button
    private lateinit var connectBtn: Button
    private lateinit var stopBtn: Button
    private lateinit var deleteBtn: Button
    private lateinit var store: ProfileStore
    private val statusHandler = Handler(Looper.getMainLooper())
    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            val text = intent?.getStringExtra(TunnelForegroundService.EXTRA_STATUS) ?: return
            updateStatus(text)
            if (text == "已连接" || text == "已断开" || text.startsWith("部分连接") || text.startsWith("连接失败") || text.startsWith("运行异常")) {
                Toast.makeText(this@MainActivity, text, Toast.LENGTH_SHORT).show()
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = ProfileStore(this)
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
            registerReceiver(statusReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            registerReceiver(statusReceiver, filter)
        }
        refreshProfileView()
        syncRuntimeStatus()
    }

    override fun onPause() {
        unregisterReceiver(statusReceiver)
        super.onPause()
    }

    override fun onDestroy() {
        statusHandler.removeCallbacksAndMessages(null)
        super.onDestroy()
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
        profileCard.addView(profileTitle)
        profileCard.addView(statusText)
        profileCard.addView(userText)
        profileCard.addView(expiryText)

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
        root.addView(View(this), LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
        addWithTop(root, configRow, dp(24))

        scroll.addView(root, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        setContentView(scroll)
    }

    private fun refreshProfileView() {
        val loaded = store.load()
        if (loaded == null) {
            userText.text = "用户：未导入"
            expiryText.text = "有效期：-"
            updateStatus("未连接", hasProfile = false)
            return
        }
        userText.text = "用户：${loaded.profile.username}"
        expiryText.text = "有效期：${formatExpiry(loaded.profile.savedCredential.expiresAt)}"
        updateStatus(TunnelForegroundService.currentStatus(this) ?: "已导入", hasProfile = true)
    }

    private fun syncRuntimeStatus() {
        val status = TunnelForegroundService.currentStatus(this) ?: return
        if (status == "已连接" || status.startsWith("部分连接") || status.startsWith("正在")) {
            startService(TunnelForegroundService.refreshIntent(this))
        }
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
                store.save(imported)
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
        val intent = TunnelForegroundService.startIntent(this)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) startForegroundService(intent) else startService(intent)
        updateStatus("正在连接")
        scheduleStatusSync()
        Toast.makeText(this, "正在连接服务端", Toast.LENGTH_SHORT).show()
    }

    private fun stopTunnel() {
        startService(TunnelForegroundService.stopIntent(this))
        updateStatus("正在断开")
        scheduleStatusSync()
        Toast.makeText(this, "正在断开连接", Toast.LENGTH_SHORT).show()
    }

    private fun deleteProfile() {
        AlertDialog.Builder(this)
            .setTitle("删除配置")
            .setMessage("删除后需要重新导入管理员下发的配置。")
            .setNegativeButton("取消", null)
            .setPositiveButton("删除") { _, _ ->
                startService(TunnelForegroundService.stopIntent(this))
                store.delete()
                refreshProfileView()
                Toast.makeText(this, "配置已删除", Toast.LENGTH_SHORT).show()
            }
            .show()
    }

    private fun updateStatus(text: String, hasProfile: Boolean = store.load() != null) {
        statusText.text = "状态：$text"
        statusText.setTextColor(statusColor(text))
        if (!::connectBtn.isInitialized) return
        val working = text.startsWith("正在")
        val connected = text == "已连接" || text.startsWith("部分连接")
        connectBtn.isEnabled = hasProfile && !working && !connected
        stopBtn.isEnabled = hasProfile && (connected || working) && text != "正在断开"
        importBtn.isEnabled = !working
        deleteBtn.isEnabled = !working
        applyButtonStyle(connectBtn, ButtonStyle.PRIMARY)
        applyButtonStyle(stopBtn, ButtonStyle.WARNING)
        applyButtonStyle(importBtn, ButtonStyle.GHOST, small = true)
        applyButtonStyle(deleteBtn, ButtonStyle.GHOST_DANGER, small = true)
    }

    private fun scheduleStatusSync() {
        listOf(700L, 1800L, 3500L).forEach { delay ->
            statusHandler.postDelayed({
                TunnelForegroundService.currentStatus(this)?.let { updateStatus(it) }
            }, delay)
        }
    }

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

    private fun statusColor(text: String): Int = when {
        text == "已连接" -> Color.rgb(0, 115, 96)
        text.startsWith("连接失败") || text.startsWith("运行异常") || text.startsWith("部分连接") -> Color.rgb(172, 96, 0)
        text.startsWith("正在") -> Color.rgb(26, 95, 139)
        else -> Color.rgb(9, 82, 76)
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
