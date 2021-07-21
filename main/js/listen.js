window.onload = function() {
    var conn;
    var playlists = "{{ . }}"; // json object
    var obj = JSON.parse(playlists);
    var log = document.getElementById("log");
    var player = document.getElementById("player"); 
    var playing = document.getElementById("playing");
    var showed = new Map();
    var msg;

    const backBtn = document.getElementById("back-btn")
    const playBtn = document.getElementById("play-btn")
    const skipBtn = document.getElementById("skip-btn")
    const play

    const goBack = () => {
        console.log("back button pushed");
        conn.send("back");
    };

    const play = () => {
        if (playBtn.value === "play"){
            conn.send("play");
            playBtn.setAttribute("value","pause")
            playBtn.innerHTML = "||"
        } else {
            conn.send("pause");
            playBtn.setAttribute("value","play")
            playBtn.innerHTML = "^"
        }
    };

    const skip = () => {
        conn.send("skip");
    };


    backBtn.addEventListener('click',goBack);
    playBtn.addEventListener('click', play);
    skipBtn.addEventListener('click',skip);

    function getMapVal(map, key) {
        return map.get(key) || 0;
    }

    function addPhoto(src) {
        return showPhoto(src,100,100,"Album Cover");
    }

    function showPhoto(src, width, height, alt) {
        var img = document.createElement("img");
        img.src = src;
        img.width = width;
        img.height = height;
        img.alt = alt;
        img.onerror = "this.onerror=null;this.src=https://dummyimage.com/100x100/000/fff";
        return img;
    }

    function appendPlaying(item) {
        debugger;
        if (playing.children.length === 3) {
            playing.removeChild(playing.childNodes[0]);
        }
        playing.appendChild(item);
        appendLog(playing);
    }

    function appendLog(item) {
        var doScroll = log.scrollTop > log.scrollHeight - log.clientHeight - 1;
        log.appendChild(item);
        if (doScroll) {
            log.scrollTop = log.scrollHeight - log.clientHeight;
        }
    }

    function initList() {
        for (let [key,value] of Object.entries(obj)) {
            scrollList(3,key,value)
        }
    }

    function scrollList(len, key, value) {
        var ind = getMapVal(showed, key);
        for (var i = ind; i < ind+len; i++) {
            var val = JSON.parse(value)
            var item = val[i];
            var li = document.createElement("li");
            showed.set(key,i);
            // create a button object
            // create list object
            console.log("Got map value of 0 for:",key);
            var img = addPhoto(item.TrackPhoto);
            console.log("Track for list:",item);
            li.appendChild(img);
            li.innerHTML = item.TrackName;
            li.value = item.TrackID;
            appendPlaying(li);
        }
    }

    // get the playback from go function
    // returns the queue object in json format
    function listen() {
        conn.send("playback");
        displayController(msg);
    }

    // get the song timestamp/length, display a line that will show the progress
    function displayController(item) {
        console.log(item);
        var p = parseFloat(item.Progress) / parseFloat(item.Duration)
        var progress = document.getElementById("bar");
        progress.setAttribute("value",string(p));
        progress.setAttribute("max","100")
        return;
    }

    if (window.WebSocket === undefined) {
        var item = document.createElement("div");
        item.innerHTML = "<b>Your browser does not support WebSockets.</b>";
        appendLog(item);
        return;
    } else {
        conn = new WebSocket("ws://" + document.location.host + "/ws");
        conn.onopen = function (evt) {
            initList();
            while (true) {
                setTimeout(listen,8500);
            }
        }
        conn.onclose = function (evt) {
            var item = document.createElement("div");
            item.innerHTML = "<b>Connection closed.</b>";
            appendLog(item);
        };
        // the messages we will receive is in the form of a json file
        // after we req data from go in respective function
        // return the message back to the function
        conn.onmessage = function (evt) {
            console.log(evt.data)
            alert("received message from server");
            msg = JSON.parse(JSON.stringify(evt.data));
        };
    }

}
