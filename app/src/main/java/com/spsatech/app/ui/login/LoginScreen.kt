package com.spsatech.app.ui.login

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.SupportAgent
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import com.spsatech.app.HelpdeskApp
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.ui.common.PasswordField
import com.spsatech.app.ui.common.repoViewModel
import kotlinx.coroutines.launch

class LoginViewModel(private val repo: HelpdeskRepository) : ViewModel() {
    var login by mutableStateOf("")
    var password by mutableStateOf("")
    var loading by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    fun submit() {
        if (loading) return
        if (login.isBlank() || password.isEmpty()) {
            error = "Введите логин и пароль"
            return
        }
        loading = true
        error = null
        viewModelScope.launch {
            // При успехе сессия сохраняется, и приложение само переключит экран.
            runCatching { repo.login(login, password) }
                .onFailure { error = it.message; password = "" }
            loading = false
        }
    }
}

@Composable
fun LoginScreen() {
    val vm = repoViewModel { LoginViewModel(it) }
    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Column(Modifier.widthIn(max = 420.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Icon(
                    Icons.Default.SupportAgent, contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(64.dp),
                )
                Text(
                    "Техподдержка", style = MaterialTheme.typography.headlineMedium,
                    modifier = Modifier.padding(top = 12.dp),
                )
                Text(
                    "Войдите с логином и паролем от «Оценки эффективности» или выданными техподдержкой",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp, bottom = 24.dp),
                )
                OutlinedTextField(
                    value = vm.login,
                    onValueChange = { vm.login = it },
                    label = { Text("Логин") },
                    singleLine = true,
                    enabled = !vm.loading,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii, imeAction = ImeAction.Next),
                    modifier = Modifier.fillMaxWidth(),
                )
                PasswordField(
                    value = vm.password,
                    onValueChange = { vm.password = it },
                    label = "Пароль",
                    enabled = !vm.loading,
                    imeAction = ImeAction.Done,
                    keyboardActions = KeyboardActions(onDone = { vm.submit() }),
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                )
                vm.error?.let {
                    Text(
                        it, color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                    )
                }
                Button(
                    onClick = vm::submit,
                    enabled = !vm.loading,
                    modifier = Modifier.fillMaxWidth().padding(top = 24.dp).height(52.dp),
                ) {
                    if (vm.loading) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                    else Text("Войти")
                }
                ServerSwitch(enabled = !vm.loading)
            }
        }
    }
}

/** Выбор сервера — только в debug-сборке, где их больше одного. */
@Composable
private fun ServerSwitch(enabled: Boolean) {
    val config = (LocalContext.current.applicationContext as HelpdeskApp).server
    if (!config.canSwitch) return
    val current by config.current.collectAsStateWithLifecycle()
    var open by remember { mutableStateOf(false) }
    Box(Modifier.padding(top = 16.dp)) {
        TextButton(onClick = { open = true }, enabled = enabled) {
            Icon(Icons.Default.Dns, contentDescription = null, modifier = Modifier.size(18.dp))
            Text("Сервер: ${current.name}", modifier = Modifier.padding(start = 6.dp))
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            config.servers.forEach { server ->
                DropdownMenuItem(
                    text = {
                        Column {
                            Text(server.name)
                            Text(
                                server.url, style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    },
                    leadingIcon = {
                        if (server.id == current.id) Icon(Icons.Default.Check, contentDescription = "Выбран")
                    },
                    onClick = { config.select(server); open = false },
                )
            }
        }
    }
}
