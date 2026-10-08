package com.spsatech.app.update

import android.app.Application
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.spsatech.app.HelpdeskApp
import kotlinx.coroutines.launch
import java.io.File

class UpdateViewModel(app: Application) : AndroidViewModel(app) {
    private val updater = (app as HelpdeskApp).updater

    var info by mutableStateOf<VersionInfo?>(null)
        private set
    var dismissed by mutableStateOf(false)
        private set
    var progress by mutableStateOf<Float?>(null)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    private var downloaded: File? = null
    private var lastCheck = 0L

    /** Не чаще раза в 30 минут: при каждом возвращении в приложение. */
    fun checkIfDue() {
        val now = System.currentTimeMillis()
        if (now - lastCheck < 30 * 60_000 || progress != null) return
        lastCheck = now
        viewModelScope.launch {
            runCatching { updater.check() }.getOrNull()?.takeIf { it.isNewer }?.let {
                if (it.versionCode != info?.versionCode) dismissed = false
                info = it
            }
        }
    }

    fun dismiss() {
        dismissed = true
    }

    /** Скачать (если ещё не скачано) и открыть системную установку. */
    fun update(launch: (android.content.Intent) -> Unit) {
        val i = info ?: return
        if (!updater.canInstall()) {
            launch(updater.installPermissionIntent())
            return
        }
        downloaded?.takeIf { it.exists() }?.let { launch(updater.installIntent(it)); return }
        if (progress != null) return
        error = null
        progress = 0f
        viewModelScope.launch {
            runCatching { updater.download(i) { p -> progress = p } }
                .onSuccess { downloaded = it; launch(updater.installIntent(it)) }
                .onFailure { error = it.message ?: "Не удалось скачать обновление" }
            progress = null
        }
    }
}

/**
 * Проверяет обновления и показывает предложение обновиться поверх [content].
 * Если текущая версия больше не поддерживается — вместо приложения экран обновления.
 */
@Composable
fun UpdateGate(content: @Composable () -> Unit) {
    val vm: UpdateViewModel = viewModel()
    val context = LocalContext.current
    LifecycleResumeEffect(Unit) {
        vm.checkIfDue()
        onPauseOrDispose { }
    }
    val info = vm.info
    val launch: (android.content.Intent) -> Unit = { context.startActivity(it) }

    if (info != null && info.isRequired) {
        RequiredUpdate(info, vm, launch)
        return
    }
    Box(Modifier.fillMaxSize()) {
        content()
        if (info != null && !vm.dismissed) {
            UpdateBanner(info, vm, launch, Modifier.align(Alignment.TopCenter))
        }
    }
}

@Composable
private fun UpdateBanner(info: VersionInfo, vm: UpdateViewModel, launch: (android.content.Intent) -> Unit, modifier: Modifier) {
    Surface(
        modifier = modifier.statusBarsPadding().padding(12.dp).widthIn(max = 520.dp).fillMaxWidth(),
        color = MaterialTheme.colorScheme.primaryContainer,
        contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        shape = MaterialTheme.shapes.large,
        shadowElevation = 6.dp,
    ) {
        Column(Modifier.padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Default.SystemUpdate, contentDescription = null)
                Text(
                    "Доступна новая версия ${info.versionName}",
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.padding(start = 12.dp).weight(1f),
                )
            }
            ProgressAndError(vm)
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = vm::dismiss, enabled = vm.progress == null) { Text("Позже") }
                TextButton(onClick = { vm.update(launch) }, enabled = vm.progress == null) { Text("Обновить") }
            }
        }
    }
}

@Composable
private fun RequiredUpdate(info: VersionInfo, vm: UpdateViewModel, launch: (android.content.Intent) -> Unit) {
    Surface(Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(
                Icons.Default.SystemUpdate, contentDescription = null,
                tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(64.dp),
            )
            Text(
                "Нужно обновить приложение", style = MaterialTheme.typography.headlineSmall,
                textAlign = TextAlign.Center, modifier = Modifier.padding(top = 16.dp),
            )
            Text(
                "Эта версия больше не поддерживается. Установите версию ${info.versionName} — " +
                    "заявки и вход сохранятся.",
                style = MaterialTheme.typography.bodyMedium, textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp, bottom = 24.dp),
            )
            ProgressAndError(vm)
            Button(
                onClick = { vm.update(launch) }, enabled = vm.progress == null,
                modifier = Modifier.fillMaxWidth().widthIn(max = 420.dp).height(52.dp),
            ) { Text("Обновить") }
        }
    }
}

@Composable
private fun ProgressAndError(vm: UpdateViewModel) {
    vm.progress?.let {
        Text(
            "Загрузка… ${(it * 100).toInt()}%", style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.padding(top = 8.dp),
        )
        LinearProgressIndicator(progress = { it }, modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp))
    }
    vm.error?.let {
        Text(
            it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.padding(top = 8.dp),
        )
    }
}
