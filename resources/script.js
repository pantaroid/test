
function check(message, callback) {
  if (window.confirm (message)) { callback(); }
}

function download(filename) {
  $("#name").val(filename);
  $("#download_form").submit();
}

function execute(params) {
  $.post("/execute", JSON.stringify(params), function(data){ document.write(data); }, "html");
}

