from flask import Flask, request, jsonify

app = Flask(__name__)


@app.route("/auth", methods=["POST"])
def auth():
    data = request.json

    if data is None:
        return jsonify({"ok": False, "id": "", "msg": "invalid json", "ttl": 0}), 400

    remote_addr = data.get("remote_addr", "")
    credential = data.get("credential", "")
    tx = data.get("tx", 0)

    if remote_addr == "123.123.123.123:5566" and credential == "wahaha" and tx == 12345:
        return jsonify({"ok": True, "id": "some_unique_id", "msg": "", "ttl": 60})
    else:
        return jsonify({"ok": False, "id": "", "msg": "invalid", "ttl": 0})


if __name__ == "__main__":
    app.run()
