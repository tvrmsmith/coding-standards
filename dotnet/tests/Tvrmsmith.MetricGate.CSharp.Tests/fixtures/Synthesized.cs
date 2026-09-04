namespace Fixtures;

// The two auto-property shapes the "compiler synthesizes a body" carve-out has to get right. A
// partial property's defining declaration is a promise, not an accessor, so only the implementing
// half is measured. A static auto-property on an interface is not abstract, so it is measured like
// any other auto-property.
public partial class Held
{
    public partial int Half { get; set; }
}

public partial class Held
{
    private int _half;

    public partial int Half
    {
        get => _half;
        set => _half = value;
    }
}

public interface ICounted
{
    static int Counter { get; set; }

    int Uncounted { get; set; }
}
