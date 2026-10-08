// Небольшие улучшения панели; без JS всё продолжает работать.
(function () {
  // Клик по строке таблицы открывает карточку.
  document.addEventListener('click', function (e) {
    var row = e.target.closest('tr[data-href]');
    if (row && !e.target.closest('a, button, input, select')) location.href = row.dataset.href;
  });

  // Просмотр фото поверх страницы.
  var box = document.getElementById('lightbox');
  document.addEventListener('click', function (e) {
    var link = e.target.closest('a[data-lightbox]');
    if (!link || e.metaKey || e.ctrlKey) return;
    e.preventDefault();
    box.querySelector('img').src = link.href;
    box.hidden = false;
  });
  if (box) {
    box.addEventListener('click', function () { box.hidden = true; });
    document.addEventListener('keydown', function (e) { if (e.key === 'Escape') box.hidden = true; });
  }

  // Подтверждение опасных действий.
  document.addEventListener('submit', function (e) {
    var msg = e.target.dataset.confirm;
    if (msg && !confirm(msg)) e.preventDefault();
  });

  // Ctrl/Cmd+Enter отправляет ответ.
  document.querySelectorAll('form[data-submit-shortcut] textarea').forEach(function (ta) {
    ta.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) ta.form.requestSubmit();
    });
  });

  // Число выбранных фото.
  document.querySelectorAll('input[data-photo-count]').forEach(function (input) {
    var label = input.form.querySelector('[data-photo-label]');
    input.addEventListener('change', function () {
      var n = input.files.length;
      if (n > 5) { alert('Можно приложить не больше 5 фото'); input.value = ''; n = 0; }
      label.textContent = n ? 'выбрано фото: ' + n : 'до 5 фото';
    });
  });

  // Копирование логина и пароля.
  document.querySelectorAll('[data-copy]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      navigator.clipboard.writeText(btn.dataset.copy).then(function () {
        btn.textContent = 'Скопировано';
      });
    });
  });

  // Очередь заявок обновляется сама, пока пользователь ничего не вводит.
  var auto = document.querySelector('[data-autorefresh]');
  if (auto) {
    setInterval(function () {
      var a = document.activeElement;
      if (document.visibilityState === 'visible' && !(a && /INPUT|TEXTAREA|SELECT/.test(a.tagName))) location.reload();
    }, Number(auto.dataset.autorefresh) * 1000);
  }
})();
