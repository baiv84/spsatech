package com.spsatech.app.ui.password

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.ui.common.PasswordField
import com.spsatech.app.ui.common.repoViewModel
import kotlinx.coroutines.launch

class ChangePasswordViewModel(private val repo: HelpdeskRepository) : ViewModel() {
    var current by mutableStateOf("")
    var new by mutableStateOf("")
    var repeat by mutableStateOf("")
    var loading by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    fun submit(onDone: () -> Unit) {
        if (loading) return
        error = when {
            current.isEmpty() || new.isEmpty() -> "Заполните все поля"
            new.length < 8 -> "Новый пароль должен быть не короче 8 символов"
            new != repeat -> "Пароли не совпадают"
            else -> null
        }
        if (error != null) return
        loading = true
        viewModelScope.launch {
            runCatching { repo.changePassword(current, new) }
                .onSuccess { onDone() }
                .onFailure { error = it.message }
            loading = false
        }
    }

    fun logout() = viewModelScope.launch { repo.logout() }
}

/**
 * Смена пароля. [forced] — пароль выдан техподдержкой и его нужно сменить
 * до начала работы: тогда вместо «Назад» показываем «Выйти».
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChangePasswordScreen(forced: Boolean, onDone: () -> Unit, onBack: () -> Unit = {}) {
    val vm = repoViewModel { ChangePasswordViewModel(it) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Смена пароля") },
                navigationIcon = {
                    if (!forced) IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Назад")
                    }
                },
                actions = {
                    if (forced) IconButton(onClick = { vm.logout() }) {
                        Icon(Icons.AutoMirrored.Filled.Logout, contentDescription = "Выйти")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
        ) {
            if (forced) {
                Text(
                    "Вы вошли с временным паролем. Придумайте новый пароль — не короче 8 символов.",
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(bottom = 16.dp),
                )
            }
            PasswordField(vm.current, { vm.current = it }, "Текущий пароль", Modifier.fillMaxWidth(), !vm.loading)
            PasswordField(
                vm.new, { vm.new = it }, "Новый пароль",
                Modifier.fillMaxWidth().padding(top = 12.dp), !vm.loading,
            )
            PasswordField(
                vm.repeat, { vm.repeat = it }, "Повторите новый пароль",
                Modifier.fillMaxWidth().padding(top = 12.dp), !vm.loading,
                imeAction = ImeAction.Done,
                keyboardActions = KeyboardActions(onDone = { vm.submit(onDone) }),
            )
            vm.error?.let {
                Text(
                    it, color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(top = 12.dp),
                )
            }
            Button(
                onClick = { vm.submit(onDone) },
                enabled = !vm.loading,
                modifier = Modifier.fillMaxWidth().padding(top = 24.dp).height(52.dp),
            ) {
                if (vm.loading) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                else Text("Сохранить")
            }
        }
    }
}
