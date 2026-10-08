package com.spsatech.app.ui.ticket

import android.content.Context
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AddPhotoAlternate
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.viewModelScope
import com.spsatech.app.HelpdeskApp
import com.spsatech.app.data.Attachment
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.data.Message
import com.spsatech.app.data.Ticket
import com.spsatech.app.data.User
import com.spsatech.app.ui.common.FullScreenLoading
import com.spsatech.app.ui.common.PhotoThumb
import com.spsatech.app.ui.common.PhotoViewer
import com.spsatech.app.ui.common.StatusChip
import com.spsatech.app.ui.common.compressPhoto
import com.spsatech.app.ui.common.formatDate
import com.spsatech.app.ui.common.repoViewModel
import com.spsatech.app.ui.newticket.MAX_PHOTOS
import kotlinx.coroutines.launch

class TicketDetailViewModel(private val repo: HelpdeskRepository, private val id: Long) : ViewModel() {
    var ticket by mutableStateOf<Ticket?>(null)
        private set
    var refreshing by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    var reply by mutableStateOf("")
    val replyPhotos = mutableStateListOf<Uri>()
    var sending by mutableStateOf(false)
        private set
    var sendError by mutableStateOf<String?>(null)
        private set

    fun refresh() {
        if (refreshing) return
        refreshing = true
        viewModelScope.launch {
            runCatching { repo.ticket(id) }
                .onSuccess { ticket = it; error = null }
                .onFailure { error = it.message }
            refreshing = false
        }
    }

    fun send(context: Context) {
        if (sending || (reply.isBlank() && replyPhotos.isEmpty())) return
        sending = true
        sendError = null
        viewModelScope.launch {
            runCatching {
                val bytes = replyPhotos.map { compressPhoto(context, it) }
                repo.addMessage(id, reply, bytes)
            }
                .onSuccess { ticket = it; reply = ""; replyPhotos.clear() }
                .onFailure { sendError = it.message ?: "Не удалось отправить" }
            sending = false
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TicketDetailScreen(id: Long, user: User, onBack: () -> Unit) {
    val vm = repoViewModel(key = "ticket-$id") { TicketDetailViewModel(it, id) }
    val context = LocalContext.current
    val app = context.applicationContext as HelpdeskApp
    var viewingRemote by rememberSaveable { mutableStateOf<Long?>(null) }
    var viewingLocal by rememberSaveable { mutableStateOf<Uri?>(null) }

    LifecycleResumeEffect(id) {
        vm.refresh()
        onPauseOrDispose { }
    }
    LaunchedEffect(id) { app.ticketUpdates.collect { if (it == id) vm.refresh() } }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Заявка №$id") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Назад")
                    }
                },
            )
        },
        bottomBar = {
            val t = vm.ticket
            if (t != null && t.status != "closed") {
                ReplyBar(vm, onViewLocal = { viewingLocal = it }, context = context)
            }
        },
        modifier = Modifier.imePadding(),
    ) { padding ->
        val t = vm.ticket
        PullToRefreshBox(
            isRefreshing = vm.refreshing && t != null,
            onRefresh = vm::refresh,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            when {
                t == null && vm.error != null -> Column(
                    Modifier.fillMaxSize().padding(32.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text(vm.error!!)
                    TextButton(onClick = vm::refresh) { Text("Повторить") }
                }
                t == null -> FullScreenLoading()
                else -> TicketContent(t, user, onViewPhoto = { viewingRemote = it.id })
            }
        }
    }

    viewingRemote?.let { PhotoViewer(app.server.attachmentUrl(it), onDismiss = { viewingRemote = null }) }
    viewingLocal?.let { PhotoViewer(it, onDismiss = { viewingLocal = null }) }
}

@Composable
private fun TicketContent(t: Ticket, user: User, onViewPhoto: (Attachment) -> Unit) {
    val listState = rememberLazyListState()
    // После нового сообщения прокручиваем вниз.
    LaunchedEffect(t.messages.size) {
        if (t.messages.isNotEmpty()) listState.animateScrollToItem(listState.layoutInfo.totalItemsCount)
    }
    LazyColumn(
        state = listState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            formatDate(t.createdAt), style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Spacer(Modifier.weight(1f))
                        StatusChip(t.status)
                    }
                    Text(t.title, style = MaterialTheme.typography.titleLarge)
                    if (t.department.isNotBlank()) Field("Подразделение", t.department)
                    if (t.location.isNotBlank()) Field("Место", t.location)
                    if (user.isStaff) Field("Автор", t.author.fullName)
                    Field("Исполнитель", t.assignee?.fullName ?: "ещё не назначен")
                    HorizontalDivider(Modifier.padding(vertical = 6.dp))
                    Text(t.description, style = MaterialTheme.typography.bodyLarge)
                    if (t.attachments.isNotEmpty()) {
                        AttachmentRow(t.attachments, onViewPhoto, Modifier.padding(top = 6.dp))
                    }
                }
            }
        }
        item {
            Text(
                if (t.messages.isEmpty()) "Техподдержка ещё не ответила" else "Переписка",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        items(t.messages, key = { it.id }) { m ->
            MessageBubble(m, mine = m.author.id == user.id, onViewPhoto = onViewPhoto)
        }
    }
}

@Composable
private fun Field(label: String, value: String) {
    Row {
        Text(
            "$label: ", style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(value, style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun AttachmentRow(items: List<Attachment>, onView: (Attachment) -> Unit, modifier: Modifier = Modifier) {
    val app = LocalContext.current.applicationContext as HelpdeskApp
    LazyRow(modifier, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        items(items, key = { it.id }) { a ->
            PhotoThumb(app.server.attachmentUrl(a.id), onClick = { onView(a) })
        }
    }
}

@Composable
private fun MessageBubble(m: Message, mine: Boolean, onViewPhoto: (Attachment) -> Unit) {
    Box(Modifier.fillMaxWidth(), contentAlignment = if (mine) Alignment.CenterEnd else Alignment.CenterStart) {
        Surface(
            color = if (mine) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceVariant,
            shape = RoundedCornerShape(
                topStart = 16.dp, topEnd = 16.dp,
                bottomStart = if (mine) 16.dp else 4.dp, bottomEnd = if (mine) 4.dp else 16.dp,
            ),
            modifier = Modifier.widthIn(max = 320.dp),
        ) {
            Column(Modifier.padding(12.dp)) {
                if (!mine) {
                    Text(
                        m.author.fullName, style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
                if (m.body.isNotBlank()) Text(m.body, style = MaterialTheme.typography.bodyMedium)
                if (m.attachments.isNotEmpty()) {
                    AttachmentRow(m.attachments, onViewPhoto, Modifier.padding(top = 6.dp))
                }
                Text(
                    formatDate(m.createdAt), style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.align(Alignment.End).padding(top = 4.dp),
                )
            }
        }
    }
}

@Composable
private fun ReplyBar(vm: TicketDetailViewModel, onViewLocal: (Uri) -> Unit, context: Context) {
    val pick = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(MAX_PHOTOS)) {
        vm.replyPhotos.addAll(it.take(MAX_PHOTOS - vm.replyPhotos.size))
    }
    Surface(tonalElevation = 3.dp) {
        Column(Modifier.padding(horizontal = 8.dp, vertical = 8.dp)) {
            vm.sendError?.let {
                Text(
                    it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                )
            }
            if (vm.replyPhotos.isNotEmpty()) {
                LazyRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                ) {
                    items(vm.replyPhotos.toList(), key = { it.toString() }) { uri ->
                        Box {
                            PhotoThumb(uri, onClick = { onViewLocal(uri) }, size = 64.dp)
                            IconButton(
                                onClick = { vm.replyPhotos.remove(uri) },
                                enabled = !vm.sending,
                                modifier = Modifier
                                    .align(Alignment.TopEnd)
                                    .padding(2.dp)
                                    .size(20.dp)
                                    .background(Color.Black.copy(alpha = 0.6f), CircleShape),
                            ) {
                                Icon(Icons.Default.Close, "Убрать фото", tint = Color.White, modifier = Modifier.size(14.dp))
                            }
                        }
                    }
                }
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                IconButton(
                    onClick = {
                        pick.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                    },
                    enabled = !vm.sending && vm.replyPhotos.size < MAX_PHOTOS,
                ) {
                    Icon(Icons.Default.AddPhotoAlternate, contentDescription = "Приложить фото")
                }
                OutlinedTextField(
                    value = vm.reply,
                    onValueChange = { vm.reply = it.take(5000) },
                    placeholder = { Text("Сообщение") },
                    enabled = !vm.sending,
                    maxLines = 4,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                    modifier = Modifier.weight(1f),
                )
                if (vm.sending) {
                    Box(Modifier.size(48.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                    }
                } else {
                    IconButton(
                        onClick = { vm.send(context.applicationContext) },
                        enabled = vm.reply.isNotBlank() || vm.replyPhotos.isNotEmpty(),
                    ) {
                        Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "Отправить")
                    }
                }
            }
        }
    }
}
