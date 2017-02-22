
// OK or NG dialog.
function check(message, callback) {
  if (window.confirm (message)) { callback(); }
}

// text input dialog.
function accept(message, callback, sample) {
  var result = prompt(message, sample);
  if (result) { callback(result); }
}


function download(filename) {
  $("#name").val(filename);
  $("#download_form").submit();
}

// execute command and refresh window.
function execute(params) {
  execute(params, null);
}

// execute command and refresh window, use callback function.
function execute(params, callback) {
  $.post("/execute", JSON.stringify(params), function(data){
    document.write(data);
    if (callback) callback();
  }, "html");
}

// execute command and redirect.
function redirect(tab, params) {
  $.post("/execute", JSON.stringify(params), function(data){ location.href = "/" + tab; });
}

$(document).ready(function() {
  var hashTabName = document.location.hash;
  console.log(hashTabName);
  if (hashTabName) {
    $('.nav-tabs a[href=' + hashTabName + ']').tab('show');
  }
});

