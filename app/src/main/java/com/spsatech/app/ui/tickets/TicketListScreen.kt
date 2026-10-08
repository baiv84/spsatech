package com.spsatech.app.ui.tickets

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Logout
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ChatBubbleOutline
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Place
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.viewModelScope
import com.spsatech.app.HelpdeskApp
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.data.TicketSummary
import com.spsatech.app.data.User
import com.spsatech.app.ui.common.FullScreenLoading
import com.spsatech.app.ui.common.StatusChip
import com.spsatech.app.ui.common.formatDate
import com.spsatech.app.ui.common.repoViewModel
import kotlinx.coroutines.launch

class TicketListViewModel(private val repo: HelpdeskRepository) : ViewModel() {
    var tickets by mutableStateOf<List<TicketSummary>?>(null)
        private set
    var refreshing by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    fun refresh() {
        if (refreshing) return
        refreshing = true
        viewModelScope.launch {
            launch { runCatching { repo.refreshMe() } }
            runCatching { repo.tickets() }
                .onSuccess { tickets = it; error = null }
                .onFailure { error = it.message }
            refreshing = false
        }
    }

    fun logout() = viewModelScope.launch { repo.logout() }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TicketListScreen(
    user: User,
    onOpen: (Long) -> Unit,
    onCreate: () -> Unit,
    onChangePassword: () -> Unit,
) {
    val vm = repoViewModel { TicketListViewModel(it) }
    // Обновляем при каждом возврате на экран — например, после создания заявки.
    LifecycleResumeEffect(Unit) {
        vm.refresh()
        onPauseOrDispose { }
    }
    val app = LocalContext.current.applicationContext as HelpdeskApp
    LaunchedEffect(Unit) { app.ticketUpdates.collect { vm.refresh() } }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (user.isStaff) "Все заявки" else "Мои заявки") },
                actions = { AccountMenu(user, onChangePassword, onLogout = { vm.logout() }) },
            )
        },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = onCreate,
                icon = { Icon(Icons.Default.Add, contentDescription = null) },
                text = { Text("Новая заявка") },
            )
        },
    ) { padding ->
        val tickets = vm.tickets
        PullToRefreshBox(
            isRefreshing = vm.refreshing && tickets != null,
            onRefresh = vm::refresh,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            when {
                tickets == null && vm.error != null -> Message(vm.error!!, action = "Повторить", onAction = vm::refresh)
                tickets == null -> FullScreenLoading()
                tickets.isEmpty() -> Message(
                    "Заявок пока нет.\nЕсли что-то не работает — создайте заявку, и техподдержка поможет."
                )
                else -> LazyColumn(
                    contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 96.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    vm.error?.let { err ->
                        item { Text(err, color = MaterialTheme.colorScheme.error) }
                    }
                    items(tickets, key = { it.id }) { t ->
                        TicketCard(t, showAuthor = user.isStaff, onClick = { onOpen(t.id) })
                    }
                }
            }
        }
    }
}

@Composable
private fun TicketCard(t: TicketSummary, showAuthor: Boolean, onClick: () -> Unit) {
    Card(Modifier.fillMaxWidth().clickable(onClick = onClick)) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "№${t.id}", style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.weight(1f))
                StatusChip(t.status)
            }
            Text(
                t.title, style = MaterialTheme.typography.titleMedium,
                maxLines = 2, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 8.dp),
            )
            if (showAuthor) {
                Text(
                    t.author.fullName, style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.padding(top = 8.dp),
            ) {
                if (t.location.isNotBlank()) {
                    Icon(Icons.Default.Place, null, Modifier.size(16.dp), MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(
                        t.location, style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1, overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(start = 2.dp).weight(1f, fill = false),
                    )
                    Spacer(Modifier.width(12.dp))
                }
                Text(
                    formatDate(t.updatedAt), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.weight(1f))
                if (t.messageCount > 0) {
                    Icon(Icons.Default.ChatBubbleOutline, null, Modifier.size(16.dp), MaterialTheme.colorScheme.primary)
                    Text(
                        "${t.messageCount}", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(start = 4.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun AccountMenu(user: User, onChangePassword: () -> Unit, onLogout: () -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }) {
            Icon(Icons.Default.AccountCircle, contentDescription = "Профиль")
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
                Text(user.fullName, style = MaterialTheme.typography.titleSmall)
                listOf(user.position, user.department).filter { it.isNotBlank() }.forEach {
                    Text(
                        it, style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                val server = (LocalContext.current.applicationContext as HelpdeskApp).server
                if (server.canSwitch) {
                    Text(
                        "Сервер: ${server.current.value.name}", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            HorizontalDivider()
            // Пароль сотрудников из «Оценки эффективности» меняется в той системе.
            if (!user.passwordManagedExternally) {
                DropdownMenuItem(
                    text = { Text("Сменить пароль") },
                    leadingIcon = { Icon(Icons.Default.Key, null) },
                    onClick = { open = false; onChangePassword() },
                )
            }
            DropdownMenuItem(
                text = { Text("Выйти") },
                leadingIcon = { Icon(Icons.AutoMirrored.Filled.Logout, null) },
                onClick = { open = false; onLogout() },
            )
        }
    }
}

/** Текст по центру экрана; прокручиваемый, чтобы работал pull-to-refresh. */
@Composable
private fun Message(text: String, action: String? = null, onAction: () -> Unit = {}) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
    Column(
        Modifier
            .verticalScroll(rememberScrollState())
            .fillMaxWidth()
            .heightIn(min = maxHeight)
            .padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            text, style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
        )
        if (action != null) TextButton(onClick = onAction) { Text(action) }
    }
    }
}
